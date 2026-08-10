package course

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ocm-backend/internal/schedule"
)

func (s *Store) scanSessionView(sc interface {
	Scan(dest ...any) error
}) (SessionView, error) {
	var v SessionView
	var date time.Time
	err := sc.Scan(
		&v.ID, &v.OfferingID, &v.ClassroomID, &date, &v.PeriodIndex, &v.Note, &v.CreatedAt,
		&v.CourseName, &v.CatalogCode, &v.TeachingClassName, &v.Teacher, &v.Semester, &v.ClassroomName,
	)
	if err == nil {
		v.Date = date.Format("2006-01-02")
	}
	return v, err
}

const sessionJoin = `
FROM course_sessions s
JOIN course_offerings o ON o.id = s.offering_id
JOIN course_catalog c ON c.id = o.catalog_id
JOIN teaching_classes tc ON tc.id = o.teaching_class_id
JOIN classrooms cr ON cr.id = s.classroom_id`

// ListSessions returns sessions matching the given filters. Zero values are
// ignored (no filter on that field).
func (s *Store) ListSessions(ctx context.Context, offeringID, classroomID int64, from, to string) ([]SessionView, error) {
	q := `SELECT s.id, s.offering_id, s.classroom_id, s.date, s.period_index, s.note, s.created_at,
       c.name, c.code, tc.name, o.teacher, o.semester, cr.name ` + sessionJoin + ` WHERE 1=1`
	var args []any
	if offeringID > 0 {
		q += ` AND s.offering_id = ?`
		args = append(args, offeringID)
	}
	if classroomID > 0 {
		q += ` AND s.classroom_id = ?`
		args = append(args, classroomID)
	}
	if from != "" {
		q += ` AND s.date >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND s.date <= ?`
		args = append(args, to)
	}
	q += ` ORDER BY s.date, s.period_index`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []SessionView
	for rows.Next() {
		v, err := s.scanSessionView(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names, err := s.classNamesBySession(ctx, sessionIDs(list))
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].ClassNames = names[list[i].ID]
		if list[i].ClassNames == nil {
			list[i].ClassNames = []string{}
		}
	}
	return list, nil
}

func (s *Store) GetSession(ctx context.Context, id int64) (SessionView, error) {
	q := `SELECT s.id, s.offering_id, s.classroom_id, s.date, s.period_index, s.note, s.created_at,
       c.name, c.code, tc.name, o.teacher, o.semester, cr.name ` + sessionJoin + ` WHERE s.id = ?`
	var v SessionView
	var date time.Time
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&v.ID, &v.OfferingID, &v.ClassroomID, &date, &v.PeriodIndex, &v.Note, &v.CreatedAt,
		&v.CourseName, &v.CatalogCode, &v.TeachingClassName, &v.Teacher, &v.Semester, &v.ClassroomName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionView{}, ErrSessionNotFound
	}
	if err != nil {
		return SessionView{}, fmt.Errorf("get session: %w", err)
	}
	v.Date = date.Format("2006-01-02")
	names, err := s.classNamesBySession(ctx, []int64{v.ID})
	if err != nil {
		return SessionView{}, err
	}
	v.ClassNames = names[v.ID]
	if v.ClassNames == nil {
		v.ClassNames = []string{}
	}
	return v, nil
}

// classNamesBySession loads the member admin class names for the given session
// IDs in one query (session -> offering -> teaching class -> members),
// returning a map keyed by session id. Used to populate SessionView.ClassNames.
func (s *Store) classNamesBySession(ctx context.Context, ids []int64) (map[int64][]string, error) {
	out := make(map[int64][]string)
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT s.id, ac.name
		 FROM course_sessions s
		 JOIN course_offerings o ON o.id = s.offering_id
		 JOIN teaching_class_members m ON m.teaching_class_id = o.teaching_class_id
		 JOIN admin_classes ac ON ac.id = m.admin_class_id
		 WHERE s.id IN (%s)
		 ORDER BY s.id, ac.grade, ac.name`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query session class names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sID int64
		var name string
		if err := rows.Scan(&sID, &name); err != nil {
			return nil, fmt.Errorf("scan session class name: %w", err)
		}
		out[sID] = append(out[sID], name)
	}
	return out, rows.Err()
}

func sessionIDs(list []SessionView) []int64 {
	ids := make([]int64, 0, len(list))
	for i := range list {
		ids = append(ids, list[i].ID)
	}
	return ids
}

func (s *Store) CreateSession(ctx context.Context, in SessionInput) (SessionView, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO course_sessions (offering_id, classroom_id, date, period_index, note) VALUES (?, ?, ?, ?, ?)`,
		in.OfferingID, in.ClassroomID, in.Date, in.PeriodIndex, in.Note,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return SessionView{}, ErrClassroomConflict
		}
		return SessionView{}, fmt.Errorf("create session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SessionView{}, fmt.Errorf("create session last insert id: %w", err)
	}
	return s.GetSession(ctx, id)
}

func (s *Store) UpdateSession(ctx context.Context, id int64, in SessionInput) (SessionView, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE course_sessions SET offering_id = ?, classroom_id = ?, date = ?, period_index = ?, note = ? WHERE id = ?`,
		in.OfferingID, in.ClassroomID, in.Date, in.PeriodIndex, in.Note, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return SessionView{}, ErrClassroomConflict
		}
		return SessionView{}, fmt.Errorf("update session: %w", err)
	}
	return s.GetSession(ctx, id)
}

func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM course_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete session rows affected: %w", err)
	}
	if n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// Timetable builds a classroom timetable grid for a date range. For each date
// it resolves the active bell-time regime and maps sessions to period slots.
func (s *Store) Timetable(ctx context.Context, classroomID int64, from, to string, regimes *schedule.Store) ([]TimetableDay, error) {
	regimeList, err := regimes.ListRegimes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list regimes for timetable: %w", err)
	}

	sessions, err := s.ListSessions(ctx, 0, classroomID, from, to)
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]map[int]SessionView)
	for _, ses := range sessions {
		m, ok := byDate[ses.Date]
		if !ok {
			m = make(map[int]SessionView)
			byDate[ses.Date] = m
		}
		m[ses.PeriodIndex] = ses
	}

	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, fmt.Errorf("invalid from date: %w", err)
	}
	toDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, fmt.Errorf("invalid to date: %w", err)
	}

	var days []TimetableDay
	for d := fromDate; !d.After(toDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		day := TimetableDay{
			Date:      dateStr,
			DayOfWeek: weekdayIndex(d),
		}
		if regime, ok := schedule.ActiveFor(regimeList, d); ok {
			day.RegimeName = regime.Name
			slots := make([]TimetableSlot, 0, len(regime.Periods))
			for _, p := range regime.Periods {
				slot := TimetableSlot{
					PeriodIndex: p.PeriodIndex,
					StartTime:   p.StartTime,
					EndTime:     p.EndTime,
				}
				if ses, ok := byDate[dateStr][p.PeriodIndex]; ok {
					s := ses
					slot.Session = &s
				}
				slots = append(slots, slot)
			}
			sort.Slice(slots, func(i, j int) bool { return slots[i].PeriodIndex < slots[j].PeriodIndex })
			day.Slots = slots
		}
		days = append(days, day)
	}
	return days, nil
}

// weekdayIndex returns 1 for Monday .. 7 for Sunday.
func weekdayIndex(d time.Time) int {
	w := int(d.Weekday()) // Sunday=0, Monday=1 ...
	if w == 0 {
		return 7
	}
	return w
}
