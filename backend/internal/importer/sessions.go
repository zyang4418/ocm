package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/schedule"
)

// Column names for the sessions import. The parser maps columns by header
// name, so column order in the file does not matter. period_start / period_end
// are shared with the bookings import (ColPeriodStart / ColPeriodEnd);
// period_end may be empty, in which case it defaults to period_start.
const (
	ColDate          = "date"
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
	periodStart       int
	periodEnd         int
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
		"periodStart":   s.periodStart,
		"periodEnd":     s.periodEnd,
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
	if rerr, ok := requireColumns(headers, ColDate, ColPeriodStart, ColClassroom, ColCourse, ColTeachingClass, ColSemester); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	// In-file overlap tracking: classroomID|date -> accepted [start,end] ranges.
	fileRanges := map[string][][2]int{}
	for i, rec := range rows {
		rowNum := i + 2 // +1 for header, +1 for 1-based
		dataRows++
		ins, rowErr := resolveSessionRow(rec, roomByID, offeringByKey, regimes)
		if rowErr != "" {
			errs = append(errs, RowError{Row: rowNum, Error: rowErr})
			continue
		}
		key := slotKey(ins.classroomID, ins.date)
		overlap := false
		for _, r := range fileRanges[key] {
			if rangesOverlap(ins.periodStart, ins.periodEnd, r[0], r[1]) {
				errs = append(errs, RowError{Row: rowNum, Error: "本文件内重复占用该教室+日期+节次"})
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		fileRanges[key] = append(fileRanges[key], [2]int{ins.periodStart, ins.periodEnd})
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
			if conflict[fmt.Sprintf("%d|%s|%d|%d", ins.classroomID, ins.date, ins.periodStart, ins.periodEnd)] {
				errs = append(errs, RowError{Row: ins.rowNum, Error: fmt.Sprintf("教室+日期+节次已被占用：%s 第%d-%d节", ins.date, ins.periodStart, ins.periodEnd)})
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
		`INSERT INTO course_sessions (offering_id, classroom_id, date, period_start, period_end, note) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		res.SucceededRows = 0
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, ins := range clean {
		if _, err := stmt.ExecContext(ctx, ins.offeringID, ins.classroomID, ins.date, ins.periodStart, ins.periodEnd, ins.note); err != nil {
			res.SucceededRows = 0
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: ins.rowNum, Error: "导入中断：" + err.Error()})
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

	periodStart, msg := parseIntField(get(ColPeriodStart), "period_start")
	if msg != "" {
		return sessionInsert{}, msg
	}
	// period_end 可选：缺省等于 period_start（单节课次）。
	periodEnd := periodStart
	if pe := strings.TrimSpace(get(ColPeriodEnd)); pe != "" {
		if periodEnd, msg = parseIntField(pe, "period_end"); msg != "" {
			return sessionInsert{}, msg
		}
	}
	if periodStart > periodEnd {
		return sessionInsert{}, "period_start 须 <= period_end"
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
	valid := schedule.PeriodIndexSet(regime)
	for p := periodStart; p <= periodEnd; p++ {
		if !valid[p] {
			return sessionInsert{}, fmt.Sprintf("节次 %d 不在该日期作息制度「%s」中", p, regime.Name)
		}
	}

	return sessionInsert{
		offeringID:        offeringID,
		classroomID:       classroomID,
		date:              dateStr,
		periodStart:       periodStart,
		periodEnd:         periodEnd,
		note:              get(ColNote),
		classroomName:     roomName,
		courseName:        courseName,
		teachingClassName: teachingClassName,
		semester:          semester,
	}, ""
}

// batchTupleSize is the number of (classroom_id, date) pairs queried per
// existingConflicts round-trip. A large 教务处 import expands to tens of
// thousands of sessions; issuing one huge IN list would risk hitting
// max_allowed_packet / parser limits. 1000 pairs (2k placeholders) per query
// stays well under any limit while keeping the round-trip count modest.
const batchTupleSize = 1000

// existingConflicts returns the set of "classroomID|date|ps|pe" keys from clean
// that overlap an existing course session or an active (pending/approved)
// booking. It queries only the (classroom, date) pairs being imported (not the
// whole table), then checks range overlap in Go. The pair set is batched
// (batchTupleSize rows per query) and the per-batch results merged, so a large
// import never builds a single huge IN list.
func existingConflicts(ctx context.Context, db *sql.DB, clean []sessionInsert) (map[string]bool, error) {
	// Collect distinct (classroomID, date) pairs being imported.
	pairSet := map[string][2]any{}
	for _, ins := range clean {
		pairSet[slotKey(ins.classroomID, ins.date)] = [2]any{ins.classroomID, ins.date}
	}
	if len(pairSet) == 0 {
		return nil, nil
	}
	pairs := make([][2]any, 0, len(pairSet))
	for _, p := range pairSet {
		pairs = append(pairs, p)
	}

	// slotKey -> occupied [ps,pe] ranges (existing sessions + active bookings).
	ranges := map[string][][2]int{}
	for start := 0; start < len(pairs); start += batchTupleSize {
		end := start + batchTupleSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch := pairs[start:end]
		placeholders := make([]string, len(batch))
		args := make([]any, 0, len(batch)*2)
		for i, p := range batch {
			placeholders[i] = "(?, ?)"
			args = append(args, p[0], p[1])
		}
		inList := strings.Join(placeholders, ",")

		sessQ := fmt.Sprintf(
			`SELECT classroom_id, date, period_start, period_end FROM course_sessions WHERE (classroom_id, date) IN (%s)`,
			inList,
		)
		sessRows, err := db.QueryContext(ctx, sessQ, args...)
		if err != nil {
			return nil, fmt.Errorf("query existing sessions: %w", err)
		}
		for sessRows.Next() {
			var cid int64
			var d time.Time
			var ps, pe int
			if err := sessRows.Scan(&cid, &d, &ps, &pe); err != nil {
				_ = sessRows.Close()
				return nil, fmt.Errorf("scan existing session: %w", err)
			}
			key := slotKey(cid, d.Format("2006-01-02"))
			ranges[key] = append(ranges[key], [2]int{ps, pe})
		}
		if err := sessRows.Err(); err != nil {
			_ = sessRows.Close()
			return nil, err
		}
		_ = sessRows.Close()

		bookQ := fmt.Sprintf(
			`SELECT classroom_id, date, period_start, period_end FROM classroom_bookings WHERE (classroom_id, date) IN (%s) AND status IN ('pending','approved')`,
			inList,
		)
		bookRows, err := db.QueryContext(ctx, bookQ, args...)
		if err != nil {
			return nil, fmt.Errorf("query existing bookings: %w", err)
		}
		for bookRows.Next() {
			var cid int64
			var d time.Time
			var ps, pe int
			if err := bookRows.Scan(&cid, &d, &ps, &pe); err != nil {
				_ = bookRows.Close()
				return nil, fmt.Errorf("scan existing booking: %w", err)
			}
			key := slotKey(cid, d.Format("2006-01-02"))
			ranges[key] = append(ranges[key], [2]int{ps, pe})
		}
		if err := bookRows.Err(); err != nil {
			_ = bookRows.Close()
			return nil, err
		}
		_ = bookRows.Close()
	}

	conflict := make(map[string]bool)
	for _, ins := range clean {
		key := slotKey(ins.classroomID, ins.date)
		for _, r := range ranges[key] {
			if rangesOverlap(ins.periodStart, ins.periodEnd, r[0], r[1]) {
				conflict[key+fmt.Sprintf("|%d|%d", ins.periodStart, ins.periodEnd)] = true
				break
			}
		}
	}
	return conflict, nil
}
