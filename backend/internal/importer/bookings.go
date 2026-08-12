package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/booking"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/schedule"
)

// Column names for the classroom_bookings import (restore mode). The layout
// matches the export so an exported file round-trips.
const (
	ColBookingClassroom = "classroom"
	ColUsername         = "username"
	ColBookingDate      = "date"
	ColPeriodStart      = "period_start"
	ColPeriodEnd        = "period_end"
	ColStatus           = "status"
	ColPurpose          = "purpose"
)

// BookingsImporter imports classroom_bookings as a restore: each row is
// inserted with the status given in the file. Rows whose status occupies the
// slot (pending/approved) are conflict-checked against existing course_sessions
// and active bookings, and deduped within the file; rejected/cancelled rows are
// inserted as historical records without conflict checks.
type BookingsImporter struct {
	db         *sql.DB
	classrooms *classroom.Store
	regimes    *schedule.Store
}

func NewBookingsImporter(db *sql.DB, classrooms *classroom.Store, regimes *schedule.Store) *BookingsImporter {
	return &BookingsImporter{db: db, classrooms: classrooms, regimes: regimes}
}

// loadRefs builds classroom name->id and username->id maps. Classroom
// availability is intentionally NOT enforced here: a restore must be able to
// recreate historical bookings for a classroom that is currently disabled. The
// returned error already carries a user-facing Chinese prefix.
func (i *BookingsImporter) loadRefs(ctx context.Context) (rooms map[string]int64, users map[string]int64, regimes0 []schedule.Regime, err error) {
	list, err := i.classrooms.List(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("加载教室列表失败：%w", err)
	}
	rooms = make(map[string]int64, len(list))
	for _, c := range list {
		rooms[strings.TrimSpace(c.Name)] = c.ID
	}
	users, err = loadUsernameMap(ctx, i.db)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("加载用户列表失败：%w", err)
	}
	regimes0, err = i.regimes.ListRegimes(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("加载作息制度失败：%w", err)
	}
	return rooms, users, regimes0, nil
}

func (i *BookingsImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	rooms, users, regimes0, err := i.loadRefs(ctx)
	if err != nil {
		return Result{Errors: []RowError{{Row: 0, Error: err.Error()}}}, nil
	}
	return analyzeBookings(ctx, i.db, rooms, users, regimes0, payload)
}

func (i *BookingsImporter) Commit(ctx context.Context, payload string) (Result, error) {
	rooms, users, regimes0, err := i.loadRefs(ctx)
	if err != nil {
		return Result{Errors: []RowError{{Row: 0, Error: err.Error()}}}, nil
	}
	return commitBookings(ctx, i.db, rooms, users, regimes0, payload)
}

// bookingInsert is a fully resolved, validated booking ready to insert.
type bookingInsert struct {
	classroomID   int64
	userID        int64
	date          string
	periodStart   int
	periodEnd     int
	status        string
	purpose       string
	classroomName string
	username      string
	rowNum        int
}

func (b bookingInsert) toPreviewMap() map[string]any {
	return map[string]any{
		"classroom":   b.classroomName,
		"username":    b.username,
		"date":        b.date,
		"periodStart": b.periodStart,
		"periodEnd":   b.periodEnd,
		"status":      b.status,
		"purpose":     b.purpose,
	}
}

// slotKey is the per-(classroom,date) bucket used for overlap checks.
func slotKey(classroomID int64, date string) string {
	return fmt.Sprintf("%d|%s", classroomID, date)
}

// rangesOverlap reports whether [ps1,pe1] overlaps [ps2,pe2].
func rangesOverlap(ps1, pe1, ps2, pe2 int) bool {
	return ps1 <= pe2 && ps2 <= pe1
}

// parseBookings parses the workbook, resolves names, validates each row, dedups
// active rows within the file, and pre-checks DB conflicts for active rows. No
// writes. A system error (e.g. the conflict query fails) is returned as err and
// aborts the whole parse.
func parseBookings(
	ctx context.Context,
	db *sql.DB,
	rooms map[string]int64,
	users map[string]int64,
	regimes []schedule.Regime,
	payload string,
) (clean []bookingInsert, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers,
		ColBookingClassroom, ColUsername, ColBookingDate, ColPeriodStart, ColPeriodEnd, ColStatus, ColPurpose); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	// In-file active overlap tracking: slotKey -> accepted (ps,pe) ranges.
	fileRanges := map[string][][2]int{}

	for i, rec := range rows {
		rowNum := i + 2
		dataRows++

		ins, rowErr := resolveBookingRow(rec, rooms, users, regimes)
		if rowErr != "" {
			errs = append(errs, RowError{Row: rowNum, Error: rowErr})
			continue
		}
		ins.rowNum = rowNum

		// Only pending/approved rows occupy the slot and need overlap checks.
		if ins.status == booking.StatusPending || ins.status == booking.StatusApproved {
			key := slotKey(ins.classroomID, ins.date)
			for _, r := range fileRanges[key] {
				if rangesOverlap(ins.periodStart, ins.periodEnd, r[0], r[1]) {
					errs = append(errs, RowError{Row: rowNum, Error: "本文件内重复占用该教室+日期+节次"})
					ins.status = "" // sentinel: skip adding to fileRanges below
					break
				}
			}
			if ins.status != "" {
				fileRanges[key] = append(fileRanges[key], [2]int{ins.periodStart, ins.periodEnd})
			}
		}
		if ins.status != "" {
			clean = append(clean, ins)
		}
	}
	if len(clean) == 0 {
		return clean, errs, dataRows, nil
	}

	// Pre-check existing course_sessions and active bookings so conflicts become
	// per-row errors instead of aborting the transaction.
	conflict, err := existingBookingConflicts(ctx, db, clean)
	if err != nil {
		errs = append(errs, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return nil, errs, dataRows, fmt.Errorf("query existing conflicts: %w", err)
	}
	if len(conflict) > 0 {
		filtered := clean[:0]
		for _, ins := range clean {
			if (ins.status == booking.StatusPending || ins.status == booking.StatusApproved) &&
				conflict[slotKey(ins.classroomID, ins.date)+fmt.Sprintf("|%d|%d", ins.periodStart, ins.periodEnd)] {
				errs = append(errs, RowError{Row: ins.rowNum, Error: fmt.Sprintf("教室+日期+节次已被占用：%s 第%d-%d节", ins.date, ins.periodStart, ins.periodEnd)})
			} else {
				filtered = append(filtered, ins)
			}
		}
		clean = filtered
	}
	return clean, errs, dataRows, nil
}

// resolveBookingRow maps one row to a bookingInsert, returning a non-empty
// error string on the first failure for that row.
func resolveBookingRow(
	rec map[string]string,
	rooms map[string]int64,
	users map[string]int64,
	regimes []schedule.Regime,
) (bookingInsert, string) {
	roomName := strings.TrimSpace(rec[ColBookingClassroom])
	if roomName == "" {
		return bookingInsert{}, "classroom 为空"
	}
	classroomID, ok := rooms[roomName]
	if !ok {
		return bookingInsert{}, "教室不存在：" + roomName
	}

	username := strings.TrimSpace(rec[ColUsername])
	if username == "" {
		return bookingInsert{}, "username 为空"
	}
	userID, ok := users[username]
	if !ok {
		return bookingInsert{}, "用户不存在：" + username
	}

	dateStr := strings.TrimSpace(rec[ColBookingDate])
	if dateStr == "" {
		return bookingInsert{}, "date 为空"
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return bookingInsert{}, "date 格式应为 YYYY-MM-DD"
	}

	ps, msg := parseIntField(rec[ColPeriodStart], "period_start")
	if msg != "" {
		return bookingInsert{}, msg
	}
	pe, msg := parseIntField(rec[ColPeriodEnd], "period_end")
	if msg != "" {
		return bookingInsert{}, msg
	}
	if ps > pe {
		return bookingInsert{}, "period_start 须 <= period_end"
	}

	status := strings.ToLower(strings.TrimSpace(rec[ColStatus]))
	if status == "" {
		status = booking.StatusPending
	}
	switch status {
	case booking.StatusPending, booking.StatusApproved, booking.StatusRejected, booking.StatusCancelled:
	default:
		return bookingInsert{}, "status 须为 pending/approved/rejected/cancelled"
	}

	purpose := strings.TrimSpace(rec[ColPurpose])
	if purpose == "" {
		return bookingInsert{}, "purpose 为空"
	}

	// Validate the period range against the active bell-time regime, mirroring
	// the booking handler. Rejected/cancelled rows still must reference a real
	// regime so the historical record is consistent.
	regime, ok := schedule.ActiveFor(regimes, date)
	if !ok {
		return bookingInsert{}, "该日期未配置作息制度"
	}
	valid := schedule.PeriodIndexSet(regime)
	for i := ps; i <= pe; i++ {
		if !valid[i] {
			return bookingInsert{}, fmt.Sprintf("节次 %d 不在该日期作息制度「%s」中", i, regime.Name)
		}
	}

	return bookingInsert{
		classroomID:   classroomID,
		userID:        userID,
		date:          dateStr,
		periodStart:   ps,
		periodEnd:     pe,
		status:        status,
		purpose:       purpose,
		classroomName: roomName,
		username:      username,
	}, ""
}

// parseIntField parses a positive integer column, returning a user-facing
// message on failure.
func parseIntField(s, name string) (int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, name + " 为空"
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, name + " 须为正整数"
	}
	return n, ""
}

func analyzeBookings(
	ctx context.Context,
	db *sql.DB,
	rooms map[string]int64,
	users map[string]int64,
	regimes []schedule.Regime,
	payload string,
) (Result, error) {
	clean, errs, dataRows, err := parseBookings(ctx, db, rooms, users, regimes, payload)
	if err != nil {
		return Result{TotalRows: dataRows, FailedRows: dataRows, Errors: errs}, err
	}
	rows := make([]map[string]any, 0, len(clean))
	for _, c := range clean {
		rows = append(rows, c.toPreviewMap())
	}
	return Result{
		TotalRows:     dataRows,
		SucceededRows: len(clean),
		FailedRows:    dataRows - len(clean),
		Errors:        errs,
		Rows:          rows,
	}, nil
}

func commitBookings(
	ctx context.Context,
	db *sql.DB,
	rooms map[string]int64,
	users map[string]int64,
	regimes []schedule.Regime,
	payload string,
) (Result, error) {
	clean, errs, dataRows, err := parseBookings(ctx, db, rooms, users, regimes, payload)
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
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO classroom_bookings (classroom_id, user_id, date, period_start, period_end, status, purpose) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, b := range clean {
		if _, err := stmt.ExecContext(ctx, b.classroomID, b.userID, b.date, b.periodStart, b.periodEnd, b.status, b.purpose); err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: b.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("insert booking: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("commit: %w", err)
	}
	res.SucceededRows = len(clean)
	res.FailedRows = dataRows - res.SucceededRows
	return res, nil
}

// loadUsernameMap returns a map from username to user id.
func loadUsernameMap(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, username FROM users`)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	m := map[string]int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		m[strings.TrimSpace(name)] = id
	}
	return m, rows.Err()
}

// existingBookingConflicts returns the set of "classroomID|date|ps|pe" keys from
// active (pending/approved) rows in clean that overlap an existing course
// session or active booking. It loads only the (classroom, date) pairs being
// imported, then checks overlap in Go.
func existingBookingConflicts(ctx context.Context, db *sql.DB, clean []bookingInsert) (map[string]bool, error) {
	// Collect distinct (classroomID, date) pairs for active rows.
	pairSet := map[string][2]any{}
	for _, b := range clean {
		if b.status != booking.StatusPending && b.status != booking.StatusApproved {
			continue
		}
		pairSet[slotKey(b.classroomID, b.date)] = [2]any{b.classroomID, b.date}
	}
	if len(pairSet) == 0 {
		return nil, nil
	}

	pairs := make([][2]any, 0, len(pairSet))
	for _, p := range pairSet {
		pairs = append(pairs, p)
	}

	// slotKey -> set of occupied period_index values (sessions).
	occupied := map[string]map[int]bool{}
	// slotKey -> list of (ps,pe) ranges (active bookings).
	ranges := map[string][][2]int{}
	// Batch the tuple IN list (batchTupleSize pairs per round-trip), mirroring
	// existingConflicts in sessions.go: a large restore with thousands of
	// distinct (classroom, date) pairs must not build one huge IN list that
	// risks max_allowed_packet / parser limits.
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

		// Sessions: period_index point occupancy.
		sessQ := fmt.Sprintf(
			`SELECT classroom_id, date, period_index FROM course_sessions WHERE (classroom_id, date) IN (%s)`,
			inList,
		)
		sessRows, err := db.QueryContext(ctx, sessQ, args...)
		if err != nil {
			return nil, fmt.Errorf("query existing sessions: %w", err)
		}
		for sessRows.Next() {
			var cid int64
			var d time.Time
			var p int
			if err := sessRows.Scan(&cid, &d, &p); err != nil {
				_ = sessRows.Close()
				return nil, fmt.Errorf("scan existing session: %w", err)
			}
			key := slotKey(cid, d.Format("2006-01-02"))
			if occupied[key] == nil {
				occupied[key] = map[int]bool{}
			}
			occupied[key][p] = true
		}
		if err := sessRows.Err(); err != nil {
			_ = sessRows.Close()
			return nil, err
		}
		_ = sessRows.Close()

		// Active bookings: range occupancy.
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

	conflict := map[string]bool{}
	for _, b := range clean {
		if b.status != booking.StatusPending && b.status != booking.StatusApproved {
			continue
		}
		key := slotKey(b.classroomID, b.date)
		hit := false
		// Session point overlap.
		for p := range occupied[key] {
			if p >= b.periodStart && p <= b.periodEnd {
				hit = true
				break
			}
		}
		// Booking range overlap.
		if !hit {
			for _, r := range ranges[key] {
				if rangesOverlap(b.periodStart, b.periodEnd, r[0], r[1]) {
					hit = true
					break
				}
			}
		}
		if hit {
			conflict[key+fmt.Sprintf("|%d|%d", b.periodStart, b.periodEnd)] = true
		}
	}
	return conflict, nil
}
