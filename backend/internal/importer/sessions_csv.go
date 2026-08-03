package importer

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/schedule"
)

// sessionInsert is a fully resolved, validated session ready to insert. The
// *Name fields mirror the resolved IDs purely for preview display.
type sessionInsert struct {
	offeringID    int64
	classroomID   int64
	date          string
	periodIndex   int
	note          string
	classroomName string
	courseName    string
	className     string
	semester      string
}

func (s sessionInsert) toPreviewRow() PreviewRow {
	return PreviewRow{
		Date:        s.date,
		PeriodIndex: s.periodIndex,
		Classroom:   s.classroomName,
		Course:      s.courseName,
		Class:       s.className,
		Semester:    s.semester,
		Note:        s.note,
	}
}

// parseAndValidate parses the CSV payload, resolves and validates each row,
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
		key := strings.TrimSpace(o.CatalogName) + "|" + strings.TrimSpace(o.ClassName) + "|" + strings.TrimSpace(o.Semester)
		offeringByKey[key] = o.ID
	}

	rows, colMap, headerErr := parseCSVHeader(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}

	seen := make(map[string]bool) // in-import dedup: classroomID|date|period
	for i, rec := range rows {
		rowNum := i + 2 // +1 for header, +1 for 1-based
		dataRows++
		ins, rowErr := resolveRow(rec, colMap, roomByID, offeringByKey, regimes)
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
				errs = append(errs, RowError{Row: 0, Error: fmt.Sprintf("教室+日期+节次已被占用：%s 第%d节", ins.date, ins.periodIndex)})
			} else {
				filtered = append(filtered, ins)
			}
		}
		clean = filtered
	}
	return clean, errs, dataRows, nil
}

// analyze is the dry-run: parse and validate, returning the rows that would be
// inserted alongside the per-row errors. No database writes.
func analyze(
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
	rows := make([]PreviewRow, 0, len(clean))
	for _, ins := range clean {
		rows = append(rows, ins.toPreviewRow())
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
// course_sessions in a single transaction. It re-validates (rather than
// trusting a prior preview) because database state may have changed between the
// preview and the commit. A transaction failure (e.g. a race with a concurrent
// insert) aborts the whole import and is returned as an error.
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

	// Atomic insert of all clean rows.
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
				res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：与已有课次冲突（并发）"})
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

// parseCSVHeader reads all CSV records, maps the first record to a column-index
// by header name, and returns the data records. A missing required column is an
// error.
func parseCSVHeader(payload string) ([][]string, map[string]int, error) {
	reader := csv.NewReader(strings.NewReader(payload))
	reader.FieldsPerRecord = -1 // lenient: missing columns become row errors
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("CSV 解析失败：%w", err)
	}
	if len(records) == 0 {
		return nil, nil, errors.New("CSV 为空")
	}
	header := records[0]
	colMap := make(map[string]int, len(header))
	for i, name := range header {
		colMap[strings.ToLower(strings.TrimSpace(name))] = i
	}
	for _, required := range []string{ColDate, ColPeriodIndex, ColClassroom, ColCourse, ColClass, ColSemester} {
		if _, ok := colMap[required]; !ok {
			return nil, nil, fmt.Errorf("表头缺少必需列：%s", required)
		}
	}
	return records[1:], colMap, nil
}

// resolveRow maps one CSV record to a sessionInsert, returning a non-empty
// error string on the first failure encountered for that row.
func resolveRow(
	rec []string,
	colMap map[string]int,
	rooms map[string]int64,
	offerings map[string]int64,
	regimes []schedule.Regime,
) (sessionInsert, string) {
	get := func(col string) (string, bool) {
		idx, ok := colMap[col]
		if !ok || idx >= len(rec) {
			return "", false
		}
		return strings.TrimSpace(rec[idx]), true
	}

	dateStr, ok := get(ColDate)
	if !ok || dateStr == "" {
		return sessionInsert{}, "date 为空"
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return sessionInsert{}, "date 格式应为 YYYY-MM-DD"
	}

	periodStr, ok := get(ColPeriodIndex)
	if !ok || periodStr == "" {
		return sessionInsert{}, "period_index 为空"
	}
	period, err := strconv.Atoi(periodStr)
	if err != nil || period < 1 {
		return sessionInsert{}, "period_index 须为正整数"
	}

	roomName, ok := get(ColClassroom)
	if !ok || roomName == "" {
		return sessionInsert{}, "classroom 为空"
	}
	classroomID, ok := rooms[roomName]
	if !ok {
		return sessionInsert{}, "教室不存在：" + roomName
	}

	courseName, ok := get(ColCourse)
	className, _ := get(ColClass)
	semester, ok2 := get(ColSemester)
	if !ok || courseName == "" || className == "" || !ok2 || semester == "" {
		return sessionInsert{}, "course / class / semester 为空"
	}
	offeringID, ok := offerings[courseName+"|"+className+"|"+semester]
	if !ok {
		return sessionInsert{}, fmt.Sprintf("开课不存在：%s / %s / %s", courseName, className, semester)
	}

	regime, ok := schedule.ActiveFor(regimes, date)
	if !ok {
		return sessionInsert{}, "该日期未配置作息制度"
	}
	valid := false
	for _, p := range regime.Periods {
		if p.PeriodIndex == period {
			valid = true
			break
		}
	}
	if !valid {
		return sessionInsert{}, fmt.Sprintf("节次 %d 不在该日期作息制度「%s」中", period, regime.Name)
	}

	note, _ := get(ColNote)
	return sessionInsert{
		offeringID:    offeringID,
		classroomID:   classroomID,
		date:          dateStr,
		periodIndex:   period,
		note:          note,
		classroomName: roomName,
		courseName:    courseName,
		className:     className,
		semester:      semester,
	}, ""
}

// existingConflicts returns a set of "classroomID|date|period" keys already
// present in course_sessions for the classrooms and date span of the clean
// rows.
func existingConflicts(ctx context.Context, db *sql.DB, clean []sessionInsert) (map[string]bool, error) {
	roomSet := make(map[int64]bool)
	var dates []string
	for _, ins := range clean {
		roomSet[ins.classroomID] = true
		dates = append(dates, ins.date)
	}
	sort.Strings(dates)
	minDate, maxDate := dates[0], dates[len(dates)-1]

	roomIDs := make([]any, 0, len(roomSet))
	for id := range roomSet {
		roomIDs = append(roomIDs, id)
	}
	placeholders := make([]string, len(roomIDs))
	for i := range roomIDs {
		placeholders[i] = "?"
	}
	q := fmt.Sprintf(
		`SELECT classroom_id, date, period_index FROM course_sessions WHERE classroom_id IN (%s) AND date BETWEEN ? AND ?`,
		strings.Join(placeholders, ","),
	)
	args := append(roomIDs, minDate, maxDate)

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
