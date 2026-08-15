package attendance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ocm-backend/internal/dbutil"
)

// RecordFilter carries the optional record-list filters. Zero values are
// ignored.
type RecordFilter struct {
	Status string
	Q      string // fuzzy contains on display name / student no
}

// PageRecords returns one page of a checkin's record list plus the total. The
// list merges two sources: the expected roster (students of the offering's
// teaching class, missing records derived as absent) and record rows outside
// the roster (InRoster=false). A standalone checkin (no offering) has an empty
// roster, so all its records come from the second source.
func (s *Store) PageRecords(ctx context.Context, checkinID int64, f RecordFilter, p dbutil.Pagination) ([]CheckinRecordView, int64, error) {
	c, err := s.GetCheckin(ctx, checkinID)
	if err != nil {
		return nil, 0, err
	}
	inner, args := recordUnionSQL(checkinID, c.OfferingID)

	where := ` WHERE 1=1`
	if f.Status != "" {
		where += ` AND t.status = ?`
		args = append(args, f.Status)
	}
	if f.Q != "" {
		where += ` AND (t.display_name LIKE ? OR t.student_no LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(f.Q))
		args = append(args, pat, pat)
	}

	total, err := dbutil.CountRows(ctx, s.db, `FROM (`+inner+`) t`+where, args)
	if err != nil {
		return nil, 0, err
	}
	q := `SELECT t.* FROM (` + inner + `) t` + where + ` ORDER BY t.extra, t.grade, t.name, t.display_name`
	q, args = p.AppendLimit(q, args)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list checkin records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	list, err := scanRecordRows(rows, c.OfferingID == 0)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListRecordsAll returns the full record list of a checkin (no pagination),
// for the single-checkin export.
func (s *Store) ListRecordsAll(ctx context.Context, checkinID int64) ([]CheckinRecordView, error) {
	list, _, err := s.PageRecords(ctx, checkinID, RecordFilter{}, dbutil.Pagination{})
	return list, err
}

// recordUnionSQL builds the UNION of roster rows and extra record rows as a
// subquery over table alias t. offeringID 0 disables both branches' roster
// joins naturally (no offering row has id 0): the roster branch yields nothing
// and every record lands in the extra branch. Bind order:
// roster(checkinID, offeringID), extra(checkinID, offeringID).
func recordUnionSQL(checkinID, offeringID int64) (string, []any) {
	roster := `
    SELECT u.id AS user_id, u.display_name, COALESCE(sp.student_no, '') AS student_no,
           ac.grade, ac.name,
           COALESCE(r.status, 'absent') AS status, r.checked_at, r.modified_at, 0 AS extra
    FROM student_profiles sp
    JOIN users u ON u.id = sp.user_id
    JOIN admin_classes ac ON ac.id = sp.admin_class_id
    JOIN teaching_class_members tcm ON tcm.admin_class_id = sp.admin_class_id
    JOIN course_offerings o ON o.teaching_class_id = tcm.teaching_class_id
    LEFT JOIN checkin_records r ON r.checkin_id = ? AND r.user_id = sp.user_id
    WHERE o.id = ?`
	extra := `
    SELECT u.id AS user_id, u.display_name, COALESCE(sp.student_no, '') AS student_no,
           COALESCE(ac.grade, ''), COALESCE(ac.name, ''),
           r.status, r.checked_at, r.modified_at, 1 AS extra
    FROM checkin_records r
    JOIN users u ON u.id = r.user_id
    LEFT JOIN student_profiles sp ON sp.user_id = r.user_id
    LEFT JOIN admin_classes ac ON ac.id = sp.admin_class_id
    WHERE r.checkin_id = ? AND NOT EXISTS (
        SELECT 1 FROM student_profiles sp2
        JOIN teaching_class_members tcm2 ON tcm2.admin_class_id = sp2.admin_class_id
        JOIN course_offerings o2 ON o2.teaching_class_id = tcm2.teaching_class_id AND o2.id = ?
        WHERE sp2.user_id = r.user_id
    )`
	return roster + ` UNION ALL ` + extra,
		[]any{checkinID, offeringID, checkinID, offeringID}
}

func scanRecordRows(rows *sql.Rows, noRoster bool) ([]CheckinRecordView, error) {
	var list []CheckinRecordView
	for rows.Next() {
		var v CheckinRecordView
		var grade, name string
		var checkedAt, modifiedAt sql.NullTime
		var extra int
		if err := rows.Scan(&v.UserID, &v.DisplayName, &v.StudentNo, &grade, &name,
			&v.Status, &checkedAt, &modifiedAt, &extra); err != nil {
			return nil, fmt.Errorf("scan checkin record: %w", err)
		}
		v.AdminClass = strings.TrimSpace(grade + " " + name)
		if checkedAt.Valid {
			t := checkedAt.Time
			v.CheckedAt = &t
		}
		if modifiedAt.Valid {
			t := modifiedAt.Time
			v.ModifiedAt = &t
		}
		v.InRoster = noRoster || extra == 0
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// UpsertRecord sets (or creates) the student's record for a checkin, used by
// teachers to correct attendance. The stored status wins over any later scan
// (scans are a no-op on existing rows). The user must exist; the checkin may
// already be closed.
func (s *Store) UpsertRecord(ctx context.Context, checkinID, userID int64, status string, modifiedBy int64) (CheckinRecordView, error) {
	c, err := s.GetCheckin(ctx, checkinID)
	if err != nil {
		return CheckinRecordView{}, err
	}
	var one int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ?`, userID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CheckinRecordView{}, ErrStudentNotFound
		}
		return CheckinRecordView{}, fmt.Errorf("lookup student: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO checkin_records (checkin_id, user_id, status, checked_at, modified_by, modified_at)
VALUES (?, ?, ?, NULL, ?, NOW())
ON DUPLICATE KEY UPDATE status = ?, modified_by = ?, modified_at = NOW()`,
		checkinID, userID, status, modifiedBy, status, modifiedBy); err != nil {
		return CheckinRecordView{}, fmt.Errorf("upsert checkin record: %w", err)
	}

	// Read back the merged row through the same shape as the list view.
	var v CheckinRecordView
	v.CheckinID = checkinID
	v.UserID = userID
	var grade, name sql.NullString
	var checkedAt, modifiedAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
SELECT u.display_name, COALESCE(sp.student_no, ''), ac.grade, ac.name, r.status, r.checked_at, r.modified_at
FROM users u
LEFT JOIN student_profiles sp ON sp.user_id = u.id
LEFT JOIN admin_classes ac ON ac.id = sp.admin_class_id
JOIN checkin_records r ON r.checkin_id = ? AND r.user_id = u.id
WHERE u.id = ?`, checkinID, userID).Scan(
		&v.DisplayName, &v.StudentNo, &grade, &name, &v.Status, &checkedAt, &modifiedAt); err != nil {
		return CheckinRecordView{}, fmt.Errorf("load upserted checkin record: %w", err)
	}
	v.AdminClass = strings.TrimSpace(grade.String + " " + name.String)
	if checkedAt.Valid {
		t := checkedAt.Time
		v.CheckedAt = &t
	}
	if modifiedAt.Valid {
		t := modifiedAt.Time
		v.ModifiedAt = &t
	}
	v.InRoster = c.OfferingID == 0
	if c.OfferingID > 0 {
		if err := s.db.QueryRowContext(ctx, rosterExistsSQL, c.OfferingID, userID).Scan(&v.InRoster); err != nil {
			return CheckinRecordView{}, fmt.Errorf("check roster: %w", err)
		}
	}
	return v, nil
}

// OfferingSummary builds the L2 semester view: every checkin of the offering
// plus one row per roster student (and per record-holding student outside the
// roster), with per-checkin statuses and per-status totals.
func (s *Store) OfferingSummary(ctx context.Context, offeringID int64) (OfferingSummary, error) {
	var out OfferingSummary
	out.OfferingID = offeringID
	if err := s.db.QueryRowContext(ctx, `
SELECT c.name, tc.name, o.semester, o.teacher
FROM course_offerings o
JOIN course_catalog c ON c.id = o.catalog_id
JOIN teaching_classes tc ON tc.id = o.teaching_class_id
WHERE o.id = ?`, offeringID).Scan(&out.CourseName, &out.TeachingClassName, &out.Semester, &out.Teacher); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OfferingSummary{}, ErrCheckinNotFound
		}
		return OfferingSummary{}, fmt.Errorf("load offering: %w", err)
	}

	checkins, _, err := s.PageCheckins(ctx, CheckinFilter{OfferingID: offeringID}, dbutil.Pagination{})
	if err != nil {
		return OfferingSummary{}, err
	}
	out.Checkins = checkins
	if len(checkins) == 0 {
		out.Rows = []SummaryRow{}
		return out, nil
	}

	// Roster rows.
	rows, err := s.db.QueryContext(ctx, `
SELECT u.id, u.display_name, COALESCE(sp.student_no, ''), ac.grade, ac.name
FROM student_profiles sp
JOIN users u ON u.id = sp.user_id
JOIN admin_classes ac ON ac.id = sp.admin_class_id
JOIN teaching_class_members tcm ON tcm.admin_class_id = sp.admin_class_id
JOIN course_offerings o ON o.teaching_class_id = tcm.teaching_class_id
WHERE o.id = ?
ORDER BY ac.grade, ac.name, u.display_name`, offeringID)
	if err != nil {
		return OfferingSummary{}, fmt.Errorf("summary roster: %w", err)
	}
	for rows.Next() {
		var userID int64
		var r SummaryRow
		var grade, name string
		if err := rows.Scan(&userID, &r.DisplayName, &r.StudentNo, &grade, &name); err != nil {
			_ = rows.Close()
			return OfferingSummary{}, fmt.Errorf("scan summary roster: %w", err)
		}
		r.UserID = userID
		r.AdminClass = strings.TrimSpace(grade + " " + name)
		r.InRoster = true
		out.Rows = append(out.Rows, r)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return OfferingSummary{}, err
	}

	// Students holding records in one of the checkins but outside the roster.
	rows, err = s.db.QueryContext(ctx, `
SELECT u.id, u.display_name, COALESCE(sp.student_no, ''), COALESCE(ac.grade, ''), COALESCE(ac.name, '')
FROM checkin_records r
JOIN checkins c ON c.id = r.checkin_id AND c.offering_id = ?
JOIN users u ON u.id = r.user_id
LEFT JOIN student_profiles sp ON sp.user_id = u.id
LEFT JOIN admin_classes ac ON ac.id = sp.admin_class_id
WHERE NOT EXISTS (
    SELECT 1 FROM student_profiles sp2
    JOIN teaching_class_members tcm2 ON tcm2.admin_class_id = sp2.admin_class_id
    JOIN course_offerings o2 ON o2.teaching_class_id = tcm2.teaching_class_id AND o2.id = ?
    WHERE sp2.user_id = r.user_id
)
GROUP BY u.id`, offeringID, offeringID)
	if err != nil {
		return OfferingSummary{}, fmt.Errorf("summary extra students: %w", err)
	}
	for rows.Next() {
		var userID int64
		var r SummaryRow
		var grade, name string
		if err := rows.Scan(&userID, &r.DisplayName, &r.StudentNo, &grade, &name); err != nil {
			_ = rows.Close()
			return OfferingSummary{}, fmt.Errorf("scan summary extra student: %w", err)
		}
		r.UserID = userID
		r.AdminClass = strings.TrimSpace(grade + " " + name)
		out.Rows = append(out.Rows, r)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return OfferingSummary{}, err
	}

	// All records of the offering's checkins, grouped into per-student maps.
	checkinIDs := make([]int64, len(checkins))
	for i := range checkins {
		checkinIDs[i] = checkins[i].ID
	}
	ph, args := placeholders(checkinIDs)
	recs := make(map[int64]map[int64]string, len(out.Rows))
	rows, err = s.db.QueryContext(ctx,
		`SELECT checkin_id, user_id, status FROM checkin_records WHERE checkin_id IN (`+ph+`)`, args...)
	if err != nil {
		return OfferingSummary{}, fmt.Errorf("summary records: %w", err)
	}
	for rows.Next() {
		var chkID, userID int64
		var status string
		if err := rows.Scan(&chkID, &userID, &status); err != nil {
			_ = rows.Close()
			return OfferingSummary{}, fmt.Errorf("scan summary record: %w", err)
		}
		m := recs[userID]
		if m == nil {
			m = map[int64]string{}
			recs[userID] = m
		}
		m[chkID] = status
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return OfferingSummary{}, err
	}

	for i := range out.Rows {
		r := &out.Rows[i]
		r.Records = map[int64]string{}
		r.Totals = map[string]int{}
		for _, c := range checkins {
			status, ok := recs[r.UserID][c.ID]
			if !ok && r.InRoster {
				status = StatusAbsent
			}
			if ok || r.InRoster {
				r.Records[c.ID] = status
				r.Totals[status]++
			}
		}
	}
	return out, nil
}

// PageMyCheckins returns one page of the student's own checkins: every checkin
// the student has a record in, with the student's status and time.
func (s *Store) PageMyCheckins(ctx context.Context, userID int64, p dbutil.Pagination) ([]MyCheckinView, int64, error) {
	from := `
FROM checkins chk
JOIN checkin_records r ON r.checkin_id = chk.id AND r.user_id = ?
LEFT JOIN course_offerings o ON o.id = chk.offering_id
LEFT JOIN course_catalog c ON c.id = o.catalog_id
LEFT JOIN teaching_classes tc ON tc.id = o.teaching_class_id
LEFT JOIN course_sessions s ON s.id = chk.session_id`
	args := []any{userID}
	total, err := dbutil.CountRows(ctx, s.db, from, args)
	if err != nil {
		return nil, 0, err
	}
	q := `SELECT chk.id, chk.title, c.name, tc.name, s.date, s.period_start, s.period_end,
       chk.starts_at, r.status, r.checked_at ` + from + ` ORDER BY chk.starts_at DESC, chk.id DESC`
	q, args = p.AppendLimit(q, args)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list my checkins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var list []MyCheckinView
	for rows.Next() {
		var v MyCheckinView
		var courseName, className sql.NullString
		var date sql.NullTime
		var ps, pe sql.NullInt64
		var checkedAt sql.NullTime
		if err := rows.Scan(&v.CheckinID, &v.Title, &courseName, &className, &date, &ps, &pe,
			&v.StartsAt, &v.Status, &checkedAt); err != nil {
			return nil, 0, fmt.Errorf("scan my checkin: %w", err)
		}
		v.CourseName = courseName.String
		v.TeachingClassName = className.String
		if date.Valid {
			v.SessionText = date.Time.Format("2006-01-02") + " " + periodText(int(ps.Int64), int(pe.Int64))
		}
		if checkedAt.Valid {
			t := checkedAt.Time
			v.CheckedAt = &t
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if list == nil {
		list = []MyCheckinView{}
	}
	return list, total, nil
}

// validRecordStatus reports whether s is one of the four record statuses.
func validRecordStatus(s string) bool {
	switch s {
	case StatusPresent, StatusAbsent, StatusLate, StatusLeave:
		return true
	}
	return false
}
