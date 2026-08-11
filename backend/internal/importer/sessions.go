package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/schedule"
)

// CSV column names for the sessions import. The parser maps columns by header
// name, so column order in the file does not matter.
const (
	ColDate          = "date"
	ColPeriodIndex   = "period_index"
	ColClassroom     = "classroom"
	ColCourse        = "course"
	ColTeachingClass = "teaching_class"
	ColSemester      = "semester"
	ColNote          = "note"
)

// SessionsImporter imports course_sessions from an xlsx file. It resolves
// classroom/offering names to IDs and validates each row against the active
// bell-time regime before inserting. Insert-only: a slot already occupied is a
// per-row error, not a silent overwrite.
type SessionsImporter struct {
	db         *sql.DB
	classrooms *classroom.Store
	courses    *course.Store
	regimes    *schedule.Store
}

func NewSessionsImporter(db *sql.DB, classrooms *classroom.Store, courses *course.Store, regimes *schedule.Store) *SessionsImporter {
	return &SessionsImporter{db: db, classrooms: classrooms, courses: courses, regimes: regimes}
}

// loadRefs fetches the reference data the importer resolves rows against. The
// returned error already carries a user-facing Chinese prefix.
func (s *SessionsImporter) loadRefs(ctx context.Context) ([]classroom.Classroom, []course.OfferingView, []schedule.Regime, error) {
	classrooms, err := s.classrooms.List(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("加载教室列表失败：%w", err)
	}
	offerings, err := s.courses.ListOfferings(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("加载开课列表失败：%w", err)
	}
	regimes, err := s.regimes.ListRegimes(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("加载作息制度失败：%w", err)
	}
	return classrooms, offerings, regimes, nil
}

func (s *SessionsImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	classrooms, offerings, regimes, err := s.loadRefs(ctx)
	if err != nil {
		return Result{Errors: []RowError{{Row: 0, Error: err.Error()}}}, nil
	}
	return analyzeSessions(ctx, s.db, classrooms, offerings, regimes, payload)
}

func (s *SessionsImporter) Commit(ctx context.Context, payload string) (Result, error) {
	classrooms, offerings, regimes, err := s.loadRefs(ctx)
	if err != nil {
		return Result{Errors: []RowError{{Row: 0, Error: err.Error()}}}, nil
	}
	return commitSessions(ctx, s.db, classrooms, offerings, regimes, payload)
}

// sessionInsert is a fully resolved, validated session ready to insert. The
// *Name fields mirror the resolved IDs purely for preview display.
type sessionInsert struct {
	offeringID        int64
	classroomID       int64
	date              string
	periodIndex       int
	note              string
	classroomName     string
	courseName        string
	teachingClassName string
	semester          string
	rowNum            int // 1-based file row, for per-row error reporting
}

func (s sessionInsert) toPreviewMap() map[string]any {
	return map[string]any{
		"date":          s.date,
		"periodIndex":   s.periodIndex,
		"classroom":     s.classroomName,
		"course":        s.courseName,
		"teachingClass": s.teachingClassName,
		"semester":      s.semester,
		"note":          s.note,
	}
}

// parseAndValidate parses the xlsx payload, resolves and validates each row,
// dedups within the file, and pre-checks existing course_sessions conflicts.
// It does not write anything. Rows that fail parsing, name resolution, period
// validation, or conflict detection are reported as per-row errors and do not
// block the rest. A system error (e.g. the conflict query fails) is returned as
// err and aborts the whole parse.
func parseAndValidate(
	ctx context.Context,
	db *sql.DB,
	classrooms []classroom.Classroom,
	offerings []course.OfferingView,
	regimes []schedule.Regime,
	payload string,
) (clean []sessionInsert, errs []RowError, dataRows int, err error) {
	roomByID := make(map[string]int64, len(classrooms))
	for _, c := range classrooms {
		roomByID[strings.TrimSpace(c.Name)] = c.ID
	}
	offeringByKey := make(map[string]int64, len(offerings))
	for _, o := range offerings {
		key := strings.TrimSpace(o.CatalogName) + "|" + strings.TrimSpace(o.TeachingClassName) + "|" + strings.TrimSpace(o.Semester)
		offeringByKey[key] = o.ID
	}

	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers, ColDate, ColPeriodIndex, ColClassroom, ColCourse, ColTeachingClass, ColSemester); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	seen := make(map[string]bool) // in-import dedup: classroomID|date|period
	for i, rec := range rows {
		rowNum := i + 2 // +1 for header, +1 for 1-based
		dataRows++
		ins, rowErr := resolveSessionRow(rec, roomByID, offeringByKey, regimes)
		if rowErr != "" {
			errs = append(errs, RowError{Row: rowNum, Error: rowErr})
			continue
		}
		key := fmt.Sprintf("%d|%s|%d", ins.classroomID, ins.date, ins.periodIndex)
		if seen[key] {
			errs = append(errs, RowError{Row: rowNum, Error: "本文件内重复占用该教室+日期+节次"})
			continue
		}
		seen[key] = true
		ins.rowNum = rowNum
		clean = append(clean, ins)
	}

	if len(clean) == 0 {
		return clean, errs, dataRows, nil
	}

	// Pre-check existing course_sessions so conflicts become per-row errors
	// instead of aborting the transaction.
	conflict, err := existingConflicts(ctx, db, clean)
	if err != nil {
		errs = append(errs, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return nil, errs, dataRows, fmt.Errorf("query existing sessions: %w", err)
	}
	if len(conflict) > 0 {
		filtered := clean[:0]
		for _, ins := range clean {
			if conflict[fmt.Sprintf("%d|%s|%d", ins.classroomID, ins.date, ins.periodIndex)] {
				errs = append(errs, RowError{Row: ins.rowNum, Error: fmt.Sprintf("教室+日期+节次已被占用：%s 第%d节", ins.date, ins.periodIndex)})
			} else {
				filtered = append(filtered, ins)
			}
		}
		clean = filtered
	}
	return clean, errs, dataRows, nil
}

// analyzeSessions is the dry-run: parse and validate, returning the rows that
// would be inserted alongside the per-row errors. No database writes.
func analyzeSessions(
	ctx context.Context,
	db *sql.DB,
	classrooms []classroom.Classroom,
	offerings []course.OfferingView,
	regimes []schedule.Regime,
	payload string,
) (Result, error) {
	clean, errs, dataRows, err := parseAndValidate(ctx, db, classrooms, offerings, regimes, payload)
	if err != nil {
		return Result{TotalRows: dataRows, FailedRows: dataRows, Errors: errs}, err
	}
	rows := make([]map[string]any, 0, len(clean))
	for _, ins := range clean {
		rows = append(rows, ins.toPreviewMap())
	}
	return Result{
		TotalRows:     dataRows,
		SucceededRows: len(clean),
		FailedRows:    dataRows - len(clean),
		Errors:        errs,
		Rows:          rows,
	}, nil
}

// commitSessions parses, validates, and inserts the valid rows into
// course_sessions in a single transaction. It re-validates (rather than trusting
// a prior preview) because database state may have changed between the preview
// and the commit. A transaction failure (e.g. a race with a concurrent insert)
// aborts the whole import and is returned as an error.
func commitSessions(
	ctx context.Context,
	db *sql.DB,
	classrooms []classroom.Classroom,
	offerings []course.OfferingView,
	regimes []schedule.Regime,
	payload string,
) (Result, error) {
	clean, errs, dataRows, err := parseAndValidate(ctx, db, classrooms, offerings, regimes, payload)
	if err != nil {
		return Result{TotalRows: dataRows, FailedRows: dataRows, Errors: errs}, err
	}

	res := Result{TotalRows: dataRows, Errors: errs}
	if len(clean) == 0 {
		res.FailedRows = dataRows
		return res, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		res.SucceededRows = 0
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO course_sessions (offering_id, classroom_id, date, period_index, note) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		res.SucceededRows = 0
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, ins := range clean {
		if _, err := stmt.ExecContext(ctx, ins.offeringID, ins.classroomID, ins.date, ins.periodIndex, ins.note); err != nil {
			res.SucceededRows = 0
			res.FailedRows = dataRows
			if isDuplicateEntry(err) {
				res.Errors = append(res.Errors, RowError{Row: ins.rowNum, Error: "导入中断：与已有课次冲突（并发）"})
				return res, errors.New("conflict during insert (race)")
			}
			res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("insert session: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		res.SucceededRows = 0
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("commit: %w", err)
	}
	res.SucceededRows = len(clean)
	res.FailedRows = dataRows - res.SucceededRows
	return res, nil
}

// resolveSessionRow maps one row to a sessionInsert, returning a non-empty error
// string on the first failure encountered for that row.
func resolveSessionRow(
	rec map[string]string,
	rooms map[string]int64,
	offerings map[string]int64,
	regimes []schedule.Regime,
) (sessionInsert, string) {
	get := func(col string) string { return rec[col] }

	dateStr := get(ColDate)
	if dateStr == "" {
		return sessionInsert{}, "date 为空"
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return sessionInsert{}, "date 格式应为 YYYY-MM-DD"
	}

	periodStr := get(ColPeriodIndex)
	if periodStr == "" {
		return sessionInsert{}, "period_index 为空"
	}
	period, err := strconv.Atoi(periodStr)
	if err != nil || period < 1 {
		return sessionInsert{}, "period_index 须为正整数"
	}

	roomName := get(ColClassroom)
	if roomName == "" {
		return sessionInsert{}, "classroom 为空"
	}
	classroomID, ok := rooms[roomName]
	if !ok {
		return sessionInsert{}, "教室不存在：" + roomName
	}

	courseName := get(ColCourse)
	teachingClassName := get(ColTeachingClass)
	semester := get(ColSemester)
	if courseName == "" || teachingClassName == "" || semester == "" {
		return sessionInsert{}, "course / teaching_class / semester 为空"
	}
	offeringID, ok := offerings[courseName+"|"+teachingClassName+"|"+semester]
	if !ok {
		return sessionInsert{}, fmt.Sprintf("开课不存在：%s / %s / %s", courseName, teachingClassName, semester)
	}

	regime, ok := schedule.ActiveFor(regimes, date)
	if !ok {
		return sessionInsert{}, "该日期未配置作息制度"
	}
	if !schedule.PeriodIndexSet(regime)[period] {
		return sessionInsert{}, fmt.Sprintf("节次 %d 不在该日期作息制度「%s」中", period, regime.Name)
	}

	return sessionInsert{
		offeringID:        offeringID,
		classroomID:       classroomID,
		date:              dateStr,
		periodIndex:       period,
		note:              get(ColNote),
		classroomName:     roomName,
		courseName:        courseName,
		teachingClassName: teachingClassName,
		semester:          semester,
	}, ""
}

// existingConflicts returns the set of "classroomID|date|period" keys from clean
// that are already present in course_sessions. It queries only the tuples being
// imported (not the whole classroom×date span), so a semester-wide import over
// many rooms/days validates a few hundred rows instead of loading tens of
// thousands.
func existingConflicts(ctx context.Context, db *sql.DB, clean []sessionInsert) (map[string]bool, error) {
	placeholders := make([]string, len(clean))
	args := make([]any, 0, len(clean)*3)
	for i, ins := range clean {
		placeholders[i] = "(?, ?, ?)"
		args = append(args, ins.classroomID, ins.date, ins.periodIndex)
	}
	q := fmt.Sprintf(
		`SELECT classroom_id, date, period_index FROM course_sessions WHERE (classroom_id, date, period_index) IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query existing sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	conflict := make(map[string]bool)
	for rows.Next() {
		var cid int64
		var d time.Time
		var p int
		if err := rows.Scan(&cid, &d, &p); err != nil {
			return nil, fmt.Errorf("scan existing session: %w", err)
		}
		conflict[fmt.Sprintf("%d|%s|%d", cid, d.Format("2006-01-02"), p)] = true
	}
	return conflict, rows.Err()
}

func isDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
