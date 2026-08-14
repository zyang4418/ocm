package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ocm-backend/internal/dbutil"
)

var (
	ErrBookingNotFound   = errors.New("booking not found")
	ErrClassroomConflict = errors.New("classroom already booked for this date and period")
	ErrInvalidTransition = errors.New("booking cannot transition from its current status")
	ErrForbidden         = errors.New("not allowed to modify this booking")
)

// Store manages booking records in the classroom_bookings table. It depends
// only on the shared *sql.DB: cross-table conflict checks (course_sessions)
// and joins (classrooms, users) query those tables directly, matching how
// course/sessions.go joins classrooms.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the classroom_bookings table. It is idempotent and safe to
// run on every startup.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS classroom_bookings (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    classroom_id  BIGINT       NOT NULL,
    user_id       BIGINT       NOT NULL,
    date          DATE         NOT NULL,
    period_start  INT          NOT NULL,
    period_end    INT          NOT NULL,
    status        VARCHAR(16)  NOT NULL DEFAULT 'pending',
    purpose       VARCHAR(255) NOT NULL DEFAULT '',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_at   TIMESTAMP    NULL DEFAULT NULL,
    INDEX idx_room_date (classroom_id, date),
    INDEX idx_user (user_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create classroom_bookings table: %w", err)
	}
	return nil
}

const selectColumns = `b.id, b.classroom_id, b.user_id, b.date, b.period_start, b.period_end, b.status, b.purpose, b.created_at, b.reviewed_at, cr.name, u.username, u.display_name`

const bookingJoin = `
FROM classroom_bookings b
JOIN classrooms cr ON cr.id = b.classroom_id
JOIN users u ON u.id = b.user_id`

// ListFilter carries the optional filters for List. Zero values are ignored.
type ListFilter struct {
	ClassroomID int64
	UserID      int64
	Status      string
	From        string // "YYYY-MM-DD", inclusive
	To          string // "YYYY-MM-DD", inclusive
}

// buildBookingWhere returns " WHERE 1=1" plus AND clauses and args for the
// non-zero ListFilter fields plus an optional fuzzy search term (purpose,
// display name, username, classroom name). Shared by List and PageBookings so
// the two cannot drift; q is always "" on the export path.
func buildBookingWhere(f ListFilter, q string) (string, []any) {
	where := ` WHERE 1=1`
	var args []any
	if f.ClassroomID > 0 {
		where += ` AND b.classroom_id = ?`
		args = append(args, f.ClassroomID)
	}
	if f.UserID > 0 {
		where += ` AND b.user_id = ?`
		args = append(args, f.UserID)
	}
	if f.Status != "" {
		where += ` AND b.status = ?`
		args = append(args, f.Status)
	}
	if f.From != "" {
		where += ` AND b.date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND b.date <= ?`
		args = append(args, f.To)
	}
	if q != "" {
		where += ` AND (b.purpose LIKE ? OR u.display_name LIKE ? OR u.username LIKE ? OR cr.name LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat, pat, pat)
	}
	return where, args
}

func (s *Store) List(ctx context.Context, f ListFilter) ([]BookingView, error) {
	where, args := buildBookingWhere(f, "")
	q := `SELECT ` + selectColumns + bookingJoin + where + ` ORDER BY b.date DESC, b.period_start DESC, b.id DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []BookingView
	for rows.Next() {
		v, err := scanBookingView(rows)
		if err != nil {
			return nil, fmt.Errorf("scan booking: %w", err)
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// PageBookings returns one page of bookings matching f plus q (fuzzy contains
// on purpose, display name, username and classroom name), plus the total
// matching count across all pages. A zero Pagination means no limit (full
// range).
func (s *Store) PageBookings(ctx context.Context, f ListFilter, q string, p dbutil.Pagination) ([]BookingView, int64, error) {
	where, args := buildBookingWhere(f, q)
	query, queryArgs := p.AppendLimit(
		`SELECT `+selectColumns+bookingJoin+where+` ORDER BY b.date DESC, b.period_start DESC, b.id DESC`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page bookings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	list := []BookingView{}
	for rows.Next() {
		v, err := scanBookingView(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan booking: %w", err)
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total, err := dbutil.CountRows(ctx, s.db, bookingJoin+where, args)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *Store) GetByID(ctx context.Context, id int64) (BookingView, error) {
	q := `SELECT ` + selectColumns + bookingJoin + ` WHERE b.id = ?`
	v, err := scanBookingView(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return BookingView{}, ErrBookingNotFound
	}
	if err != nil {
		return BookingView{}, fmt.Errorf("get booking: %w", err)
	}
	return v, nil
}

// Create inserts a pending booking. The classroom row is locked for the
// duration of an inner transaction so that concurrent Create calls for the
// same classroom serialize: a second caller's conflict check runs only after
// the first commits, by which point the inserted booking is visible and
// reported as a conflict. The check itself verifies that no course session
// and no active (pending/approved) booking overlaps the requested period
// range for this classroom and date.
func (s *Store) Create(ctx context.Context, in BookingInput, userID int64) (BookingView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BookingView{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the classrooms row (guaranteed to exist by validateBooking) and
	// hold the X lock until commit. This is the lock anchor rather than a
	// classroom_bookings row: FOR UPDATE on an empty result set acquires
	// only a gap lock, which does not block a concurrent INSERT into that
	// gap, so locking the target booking row would not serialize inserts
	// into a still-empty slot.
	var lockID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM classrooms WHERE id = ? FOR UPDATE`, in.ClassroomID).Scan(&lockID); err != nil {
		return BookingView{}, fmt.Errorf("lock classroom: %w", err)
	}

	if conflict, err := conflicts(ctx, tx, in.ClassroomID, in.Date, in.PeriodStart, in.PeriodEnd, 0); err != nil {
		return BookingView{}, err
	} else if conflict {
		return BookingView{}, ErrClassroomConflict
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO classroom_bookings (classroom_id, user_id, date, period_start, period_end, status, purpose) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.ClassroomID, userID, in.Date, in.PeriodStart, in.PeriodEnd, StatusPending, in.Purpose,
	)
	if err != nil {
		return BookingView{}, fmt.Errorf("create booking: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return BookingView{}, fmt.Errorf("create booking last insert id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BookingView{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetByID(ctx, id)
}

// Cancel sets a booking to cancelled. Only the booker (or an admin) may cancel,
// and only a pending or approved booking can be cancelled.
func (s *Store) Cancel(ctx context.Context, id, userID int64, isAdmin bool) (BookingView, error) {
	v, err := s.GetByID(ctx, id)
	if err != nil {
		return BookingView{}, err
	}
	if !isAdmin && v.UserID != userID {
		return BookingView{}, ErrForbidden
	}
	if v.Status != StatusPending && v.Status != StatusApproved {
		return BookingView{}, ErrInvalidTransition
	}
	// Condition the update on the current status so a concurrent transition
	// (e.g. an admin rejecting it between the read above and this write)
	// cannot be overwritten. affected == matched here because the status
	// column always changes value on a successful cancel, so the
	// changed-rows vs matched-rows distinction does not apply.
	res, err := s.db.ExecContext(ctx,
		`UPDATE classroom_bookings SET status = ? WHERE id = ? AND status IN (?, ?)`,
		StatusCancelled, id, StatusPending, StatusApproved,
	)
	if err != nil {
		return BookingView{}, fmt.Errorf("cancel booking: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return BookingView{}, fmt.Errorf("cancel booking rows affected: %w", err)
	}
	if n == 0 {
		return BookingView{}, ErrInvalidTransition
	}
	return s.GetByID(ctx, id)
}

// Review applies an admin's approve/reject decision to a pending booking. On
// approve it re-checks for conflicts (a session or another booking may have
// taken the slot since the request was made) and refuses with
// ErrClassroomConflict if the slot is no longer free.
func (s *Store) Review(ctx context.Context, id int64, decision string) (BookingView, error) {
	v, err := s.GetByID(ctx, id)
	if err != nil {
		return BookingView{}, err
	}
	if v.Status != StatusPending {
		return BookingView{}, ErrInvalidTransition
	}

	newStatus := StatusRejected
	if decision == "approve" {
		newStatus = StatusApproved
	}

	if newStatus == StatusApproved {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return BookingView{}, fmt.Errorf("begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		if conflict, err := conflicts(ctx, tx, v.ClassroomID, v.Date, v.PeriodStart, v.PeriodEnd, id); err != nil {
			return BookingView{}, err
		} else if conflict {
			return BookingView{}, ErrClassroomConflict
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE classroom_bookings SET status = ?, reviewed_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`,
			newStatus, id, StatusPending,
		)
		if err != nil {
			return BookingView{}, fmt.Errorf("approve booking: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return BookingView{}, fmt.Errorf("approve booking rows affected: %w", err)
		}
		if n == 0 {
			// A concurrent transition changed the status away from pending
			// between the fast-path read and this update; the deferred
			// rollback discards the no-op transaction.
			return BookingView{}, ErrInvalidTransition
		}
		if err := tx.Commit(); err != nil {
			return BookingView{}, fmt.Errorf("commit: %w", err)
		}
	} else {
		res, err := s.db.ExecContext(ctx,
			`UPDATE classroom_bookings SET status = ?, reviewed_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`,
			newStatus, id, StatusPending,
		)
		if err != nil {
			return BookingView{}, fmt.Errorf("reject booking: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return BookingView{}, fmt.Errorf("reject booking rows affected: %w", err)
		}
		if n == 0 {
			return BookingView{}, ErrInvalidTransition
		}
	}
	return s.GetByID(ctx, id)
}

// conflicts reports whether any course session or active booking overlaps the
// period range [ps, pe] for the given classroom and date. excludeID omits a
// booking from the overlap check (used when approving an existing booking).
func conflicts(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, classroomID int64, date string, ps, pe int, excludeID int64) (bool, error) {
	var one int

	err := q.QueryRowContext(ctx,
		`SELECT 1 FROM course_sessions WHERE classroom_id = ? AND date = ? AND period_start <= ? AND period_end >= ? LIMIT 1`,
		classroomID, date, pe, ps,
	).Scan(&one)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("check session conflict: %w", err)
	}

	bookingQuery := `SELECT 1 FROM classroom_bookings WHERE classroom_id = ? AND date = ? AND status IN ('pending','approved') AND period_start <= ? AND period_end >= ?`
	args := []any{classroomID, date, pe, ps}
	if excludeID > 0 {
		bookingQuery += ` AND id <> ?`
		args = append(args, excludeID)
	}
	bookingQuery += ` LIMIT 1`

	err = q.QueryRowContext(ctx, bookingQuery, args...).Scan(&one)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("check booking conflict: %w", err)
	}
	return false, nil
}

func scanBookingView(sc interface {
	Scan(dest ...any) error
}) (BookingView, error) {
	var v BookingView
	var date time.Time
	var reviewed sql.NullTime
	err := sc.Scan(
		&v.ID, &v.ClassroomID, &v.UserID, &date, &v.PeriodStart, &v.PeriodEnd, &v.Status, &v.Purpose, &v.CreatedAt, &reviewed,
		&v.ClassroomName, &v.Username, &v.DisplayName,
	)
	if err == nil {
		v.Date = date.Format("2006-01-02")
		if reviewed.Valid {
			t := reviewed.Time
			v.ReviewedAt = &t
		}
	}
	return v, err
}
