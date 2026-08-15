package attendance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"ocm-backend/internal/dbutil"
)

// Store manages the checkins and checkin_records tables. Offering/session
// references are logical foreign keys validated against the course tables by
// direct lookup (same pattern as course.Store.teachingClassExists) so this
// package stays independent of the course package.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the checkin tables. It is idempotent and safe to run on
// every startup.
func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS checkins (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    offering_id  BIGINT       NULL DEFAULT NULL,
    session_id   BIGINT       NULL DEFAULT NULL,
    title        VARCHAR(128) NOT NULL,
    code         VARCHAR(6)   NOT NULL,
    late_minutes INT          NOT NULL DEFAULT 0,
    status       VARCHAR(16)  NOT NULL DEFAULT 'active',
    starts_at    DATETIME     NOT NULL,
    expires_at   DATETIME     NULL DEFAULT NULL,
    created_by   BIGINT       NOT NULL,
    created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at    DATETIME     NULL DEFAULT NULL,
    UNIQUE KEY uq_checkin_code (code),
    KEY idx_checkin_offering (offering_id),
    KEY idx_checkin_session (session_id),
    KEY idx_checkin_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS checkin_records (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    checkin_id  BIGINT       NOT NULL,
    user_id     BIGINT       NOT NULL,
    status      VARCHAR(16)  NOT NULL DEFAULT 'present',
    checked_at  DATETIME     NULL DEFAULT NULL,
    modified_by BIGINT       NULL DEFAULT NULL,
    modified_at DATETIME     NULL DEFAULT NULL,
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_checkin_user (checkin_id, user_id),
    KEY idx_record_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create attendance table: %w", err)
		}
	}
	return nil
}

// ---- Checkin CRUD ----

// CreateCheckin inserts a checkin and returns its view. Offering/session links
// are validated first (offering exists; session exists and belongs to the
// offering); a standalone checkin (no offering) requires a title. The code is
// a random 6-digit string, retried on the rare 1062 collision.
func (s *Store) CreateCheckin(ctx context.Context, in CheckinInput, createdBy int64) (CheckinView, error) {
	if in.OfferingID == 0 && strings.TrimSpace(in.Title) == "" {
		return CheckinView{}, ErrTitleRequired
	}
	if in.LateMinutes < 0 {
		in.LateMinutes = 0
	}

	title := strings.TrimSpace(in.Title)
	if in.SessionID > 0 {
		var offeringID int64
		var date time.Time
		var ps, pe int
		var courseName, className string
		if err := s.db.QueryRowContext(ctx, `
SELECT s.offering_id, s.date, s.period_start, s.period_end, c.name, tc.name
FROM course_sessions s
JOIN course_offerings o ON o.id = s.offering_id
JOIN course_catalog c ON c.id = o.catalog_id
JOIN teaching_classes tc ON tc.id = o.teaching_class_id
WHERE s.id = ?`, in.SessionID).Scan(&offeringID, &date, &ps, &pe, &courseName, &className); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return CheckinView{}, fmt.Errorf("session %d: %w", in.SessionID, ErrCheckinNotFound)
			}
			return CheckinView{}, fmt.Errorf("load session: %w", err)
		}
		if in.OfferingID == 0 {
			in.OfferingID = offeringID
		} else if offeringID != in.OfferingID {
			return CheckinView{}, ErrOfferingMismatch
		}
		if title == "" {
			title = fmt.Sprintf("%s %s %s %s", courseName, className, date.Format("2006-01-02"), periodText(ps, pe))
		}
	} else if in.OfferingID > 0 {
		var courseName, className string
		if err := s.db.QueryRowContext(ctx, `
SELECT c.name, tc.name
FROM course_offerings o
JOIN course_catalog c ON c.id = o.catalog_id
JOIN teaching_classes tc ON tc.id = o.teaching_class_id
WHERE o.id = ?`, in.OfferingID).Scan(&courseName, &className); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return CheckinView{}, fmt.Errorf("offering %d: %w", in.OfferingID, ErrCheckinNotFound)
			}
			return CheckinView{}, fmt.Errorf("load offering: %w", err)
		}
		if title == "" {
			title = courseName + " " + className
		}
	}

	now := time.Now()
	var expiresAt *time.Time
	if in.DurationMinute > 0 {
		t := now.Add(time.Duration(in.DurationMinute) * time.Minute)
		expiresAt = &t
	}

	var id int64
	for attempt := 0; ; attempt++ {
		code := fmt.Sprintf("%06d", rand.Intn(1000000))
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO checkins (offering_id, session_id, title, code, late_minutes, status, starts_at, expires_at, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nullIfZero(in.OfferingID), nullIfZero(in.SessionID), title, code, in.LateMinutes,
			StatusActive, now, expiresAt, createdBy)
		if err != nil {
			if dbutil.IsDuplicateEntry(err) && attempt < 10 {
				continue // 6-digit collision; retry with a fresh code
			}
			return CheckinView{}, fmt.Errorf("create checkin: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return CheckinView{}, fmt.Errorf("create checkin last insert id: %w", err)
		}
		break
	}
	return s.GetCheckin(ctx, id)
}

// GetCheckin returns one checkin view with counts.
func (s *Store) GetCheckin(ctx context.Context, id int64) (CheckinView, error) {
	views, err := s.getCheckins(ctx, `WHERE chk.id = ?`, id)
	if err != nil {
		return CheckinView{}, err
	}
	if len(views) == 0 {
		return CheckinView{}, ErrCheckinNotFound
	}
	return views[0], nil
}

// CheckinFilter carries the optional list filters. Zero values are ignored.
type CheckinFilter struct {
	OfferingID int64
	SessionID  int64
	Status     string
	From       string // "YYYY-MM-DD", inclusive on starts_at
	To         string // "YYYY-MM-DD", inclusive
	Q          string // fuzzy contains on title/course/teaching class
}

// PageCheckins returns one page of checkin views plus the total matching
// count. A zero Pagination means no limit (full range, used by exports).
func (s *Store) PageCheckins(ctx context.Context, f CheckinFilter, p dbutil.Pagination) ([]CheckinView, int64, error) {
	where, args := buildCheckinWhere(f)
	total, err := dbutil.CountRows(ctx, s.db, `FROM checkins chk`+checkinJoins+where, args)
	if err != nil {
		return nil, 0, err
	}
	q := checkinSelect + `
FROM checkins chk` + checkinJoins + where + ` ORDER BY chk.starts_at DESC, chk.id DESC`
	q, args = p.AppendLimit(q, args)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list checkins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	list, err := s.scanCheckins(rows)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CloseCheckin ends an active checkin. The status transition is atomic: only
// an active row can be closed, so concurrent closes and scans cannot interleave.
func (s *Store) CloseCheckin(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE checkins SET status = ?, closed_at = NOW() WHERE id = ? AND status = ?`,
		StatusClosed, id, StatusActive)
	if err != nil {
		return fmt.Errorf("close checkin: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("close checkin rows affected: %w", err)
	}
	if n == 0 {
		var one int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM checkins WHERE id = ?`, id).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrCheckinNotFound
			}
			return fmt.Errorf("close checkin lookup: %w", err)
		}
		return ErrCheckinNotActive
	}
	return nil
}

// ---- Scan ----

// ScanByCode records the student's attendance for the checkin matching code.
// The checkin row is locked so scans serialize against closes: a checkin
// closed (or expired, which auto-closes) rejects the scan. Duplicate scans are
// idempotent — the first checked_at and status win, so a teacher correction is
// never overwritten by a later scan.
func (s *Store) ScanByCode(ctx context.Context, code string, userID int64) (ScanResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScanResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		id       int64
		status   string
		startsAt time.Time
		lateMin  int
		title    string
		offering sql.NullInt64
		expires  sql.NullTime
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id, offering_id, title, status, starts_at, late_minutes, expires_at
FROM checkins WHERE code = ? FOR UPDATE`, code).
		Scan(&id, &offering, &title, &status, &startsAt, &lateMin, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return ScanResult{}, ErrCodeNotFound
	}
	if err != nil {
		return ScanResult{}, fmt.Errorf("scan checkin lookup: %w", err)
	}
	if status != StatusActive {
		return ScanResult{}, ErrCheckinNotActive
	}
	if expires.Valid && time.Now().After(expires.Time) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE checkins SET status = ?, closed_at = NOW() WHERE id = ?`,
			StatusClosed, id); err != nil {
			return ScanResult{}, fmt.Errorf("auto-close expired checkin: %w", err)
		}
		return ScanResult{}, ErrCheckinExpired
	}

	now := time.Now()
	recordStatus := StatusPresent
	if lateMin > 0 && now.After(startsAt.Add(time.Duration(lateMin)*time.Minute)) {
		recordStatus = StatusLate
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO checkin_records (checkin_id, user_id, status, checked_at) VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE checked_at = checked_at, status = status`,
		id, userID, recordStatus, now)
	if err != nil {
		return ScanResult{}, fmt.Errorf("insert checkin record: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ScanResult{}, fmt.Errorf("insert checkin record rows affected: %w", err)
	}
	isNew := n == 1
	if !isNew {
		if err := tx.QueryRowContext(ctx,
			`SELECT status FROM checkin_records WHERE checkin_id = ? AND user_id = ?`,
			id, userID).Scan(&recordStatus); err != nil {
			return ScanResult{}, fmt.Errorf("load existing checkin record: %w", err)
		}
	}

	inRoster := true
	if offering.Valid && offering.Int64 > 0 {
		if err := tx.QueryRowContext(ctx, rosterExistsSQL, offering.Int64, userID).
			Scan(&inRoster); err != nil {
			return ScanResult{}, fmt.Errorf("check roster: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ScanResult{}, fmt.Errorf("commit: %w", err)
	}
	return ScanResult{CheckinID: id, Title: title, Status: recordStatus, IsNew: isNew, InRoster: inRoster}, nil
}

// ---- View building ----

const checkinSelect = `SELECT chk.id, chk.offering_id, chk.session_id, chk.title, chk.code, chk.late_minutes,
       chk.status, chk.starts_at, chk.expires_at, chk.created_by, chk.created_at, chk.closed_at,
       c.name, tc.name, o.semester, o.teacher, s.date, s.period_start, s.period_end `

const checkinJoins = `
LEFT JOIN course_offerings o ON o.id = chk.offering_id
LEFT JOIN course_catalog c ON c.id = o.catalog_id
LEFT JOIN teaching_classes tc ON tc.id = o.teaching_class_id
LEFT JOIN course_sessions s ON s.id = chk.session_id
`

// rosterExistsSQL reports whether the student belongs to the offering's
// teaching class via student_profiles → admin_classes → teaching_class_members.
const rosterExistsSQL = `
SELECT EXISTS(
    SELECT 1 FROM student_profiles sp
    JOIN teaching_class_members tcm ON tcm.admin_class_id = sp.admin_class_id
    JOIN course_offerings o ON o.teaching_class_id = tcm.teaching_class_id
    WHERE o.id = ? AND sp.user_id = ?
)`

// rosterCountSQL counts distinct roster students of one offering.
const rosterCountSQL = `
SELECT COUNT(DISTINCT sp.user_id)
FROM student_profiles sp
JOIN teaching_class_members tcm ON tcm.admin_class_id = sp.admin_class_id
JOIN course_offerings o ON o.teaching_class_id = tcm.teaching_class_id
WHERE o.id = ?`

func buildCheckinWhere(f CheckinFilter) (string, []any) {
	where := ` WHERE 1=1`
	var args []any
	if f.OfferingID > 0 {
		where += ` AND chk.offering_id = ?`
		args = append(args, f.OfferingID)
	}
	if f.SessionID > 0 {
		where += ` AND chk.session_id = ?`
		args = append(args, f.SessionID)
	}
	if f.Status != "" {
		where += ` AND chk.status = ?`
		args = append(args, f.Status)
	}
	if f.From != "" {
		where += ` AND chk.starts_at >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND chk.starts_at < ? + INTERVAL 1 DAY`
		args = append(args, f.To)
	}
	if f.Q != "" {
		where += ` AND (chk.title LIKE ? OR c.name LIKE ? OR tc.name LIKE ? OR o.semester LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(f.Q))
		args = append(args, pat, pat, pat, pat)
	}
	return where, args
}

// getCheckins runs a checkin-select query built from where/args, backfills
// counts for the returned rows and returns the views.
func (s *Store) getCheckins(ctx context.Context, where string, args ...any) ([]CheckinView, error) {
	rows, err := s.db.QueryContext(ctx, checkinSelect+`
FROM checkins chk`+checkinJoins+where, args...)
	if err != nil {
		return nil, fmt.Errorf("get checkins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	list, err := s.scanCheckins(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return list, nil
	}
	ids := make([]int64, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	if err := s.backfillCounts(ctx, list, ids); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) scanCheckins(rows *sql.Rows) ([]CheckinView, error) {
	var list []CheckinView
	for rows.Next() {
		var v CheckinView
		var offeringID, sessionID sql.NullInt64
		var semester, teacher, courseName, className sql.NullString
		var date sql.NullTime
		var ps, pe sql.NullInt64
		var expires, closed sql.NullTime
		if err := rows.Scan(
			&v.ID, &offeringID, &sessionID, &v.Title, &v.Code, &v.LateMinutes,
			&v.Status, &v.StartsAt, &expires, &v.CreatedBy, &v.CreatedAt, &closed,
			&courseName, &className, &semester, &teacher, &date, &ps, &pe,
		); err != nil {
			return nil, fmt.Errorf("scan checkin: %w", err)
		}
		v.OfferingID = offeringID.Int64
		v.SessionID = sessionID.Int64
		v.CourseName = courseName.String
		v.TeachingClassName = className.String
		v.Semester = semester.String
		v.Teacher = teacher.String
		if expires.Valid {
			t := expires.Time
			v.ExpiresAt = &t
		}
		if closed.Valid {
			t := closed.Time
			v.ClosedAt = &t
		}
		if date.Valid {
			v.SessionText = date.Time.Format("2006-01-02") + " " + periodText(int(ps.Int64), int(pe.Int64))
		}
		if v.Status == StatusActive && v.ExpiresAt != nil && time.Now().After(*v.ExpiresAt) {
			v.Status = StatusClosed
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// backfillCounts fills list[i].Counts for the given checkin ids with grouped
// queries (records by status, roster size per offering-linked checkin).
// Absent = explicit-absent records + roster students without any record row.
func (s *Store) backfillCounts(ctx context.Context, list []CheckinView, ids []int64) error {
	type stat struct {
		present, late, absent, leave int
		expected, rosterRows         int
	}
	stats := make(map[int64]*stat, len(ids))
	for _, id := range ids {
		stats[id] = &stat{}
	}

	// Records by status, across all given checkins.
	ph, args := placeholders(ids)
	rows, err := s.db.QueryContext(ctx,
		`SELECT checkin_id, status, COUNT(*) FROM checkin_records
WHERE checkin_id IN (`+ph+`) GROUP BY checkin_id, status`, args...)
	if err != nil {
		return fmt.Errorf("checkin record counts: %w", err)
	}
	for rows.Next() {
		var id int64
		var status string
		var n int
		if err := rows.Scan(&id, &status, &n); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan record counts: %w", err)
		}
		st := stats[id]
		switch status {
		case StatusPresent:
			st.present = n
		case StatusLate:
			st.late = n
		case StatusAbsent:
			st.absent = n
		case StatusLeave:
			st.leave = n
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Per offering: roster size and, per checkin, how many roster students
	// already have a record row. Absent = explicit + expected - rosterRows.
	offeringIDs := map[int64][]int64{} // offeringID -> checkin ids
	for _, v := range list {
		if v.OfferingID > 0 {
			offeringIDs[v.OfferingID] = append(offeringIDs[v.OfferingID], v.ID)
		}
	}
	for offeringID, chkIDs := range offeringIDs {
		var expected int
		if err := s.db.QueryRowContext(ctx, rosterCountSQL, offeringID).Scan(&expected); err != nil {
			return fmt.Errorf("roster count: %w", err)
		}
		ph, args := placeholders(chkIDs)
		args = append(args, offeringID)
		rows, err := s.db.QueryContext(ctx,
			`SELECT r.checkin_id, COUNT(*)
FROM checkin_records r
JOIN student_profiles sp ON sp.user_id = r.user_id
JOIN teaching_class_members tcm ON tcm.admin_class_id = sp.admin_class_id
JOIN course_offerings o ON o.teaching_class_id = tcm.teaching_class_id
WHERE r.checkin_id IN (`+ph+`) AND o.id = ?
GROUP BY r.checkin_id`, args...)
		if err != nil {
			return fmt.Errorf("roster record counts: %w", err)
		}
		for rows.Next() {
			var chkID int64
			var n int
			if err := rows.Scan(&chkID, &n); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan roster record counts: %w", err)
			}
			stats[chkID].rosterRows = n
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, chkID := range chkIDs {
			stats[chkID].expected = expected
		}
	}

	for i := range list {
		st := stats[list[i].ID]
		st.absent += st.expected - st.rosterRows
		list[i].Counts = Counts{
			Expected: st.expected, Present: st.present, Late: st.late, Absent: st.absent, Leave: st.leave,
		}
	}
	return nil
}

// placeholders returns a comma-joined "?" list for len(ids) and the ids as
// query args, for IN (...) clauses.
func placeholders(ids []int64) (string, []any) {
	parts := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		parts[i] = "?"
		args[i] = id
	}
	return strings.Join(parts, ","), args
}

func nullIfZero(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func periodText(ps, pe int) string {
	if ps <= 0 {
		return ""
	}
	if pe <= 0 || pe == ps {
		return fmt.Sprintf("第%d节", ps)
	}
	return fmt.Sprintf("第%d-%d节", ps, pe)
}
