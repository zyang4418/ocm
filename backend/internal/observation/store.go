package observation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ocm-backend/internal/dbutil"
)

var (
	// ErrSessionMismatch reports that an occurrence/session does not belong to
	// the requested course offering.
	ErrSessionMismatch = errors.New("session does not belong to the course offering")
	// ErrClassroomMismatch reports that a classroom does not match the session's.
	ErrClassroomMismatch = errors.New("classroom does not match the session")
)

// Store manages observation records in the observations table. It depends only
// on the shared *sql.DB; cross-entity lookups (offerings, sessions, classrooms,
// users) query those tables directly, matching how booking and course join the
// shared tables.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Migrate creates the observations table. It is idempotent and safe to run on
// every startup.
//
// Uniqueness mirrors the legacy partial constraints via MySQL's "NULLs are
// distinct in a UNIQUE index" behaviour: sections_key is NULL whenever an
// occurrence is set, so uq_observer_occurrence only constrains occurrence rows
// and uq_observer_course_date_sections only constrains occurrence-less rows —
// exactly the two disjoint cases the legacy model expressed.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS observations (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    template_type   VARCHAR(32)  NOT NULL DEFAULT '',
    occurrence_id   BIGINT       NULL DEFAULT NULL,
    course_id       BIGINT       NOT NULL,
    classroom_id    BIGINT       NULL DEFAULT NULL,
    observer_id     BIGINT       NOT NULL,
    observe_date    DATE         NOT NULL,
    sections        LONGTEXT     NOT NULL,
    sections_key    VARCHAR(64)  NULL DEFAULT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'draft',
    scores          LONGTEXT     NOT NULL,
    total_score     DOUBLE       NULL DEFAULT NULL,
    content         LONGTEXT     NOT NULL,
    form_data       LONGTEXT     NOT NULL,
    course_snapshot LONGTEXT     NOT NULL,
    is_anonymous    TINYINT(1)   NOT NULL DEFAULT 0,
    remark          TEXT         NOT NULL,
    exported_at     TIMESTAMP    NULL DEFAULT NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_observer_occurrence (observer_id, occurrence_id),
    UNIQUE KEY uq_observer_course_date_sections (observer_id, course_id, observe_date, sections_key),
    INDEX idx_observer_date (observer_id, observe_date),
    INDEX idx_course_date (course_id, observe_date),
    INDEX idx_classroom_date (classroom_id, observe_date),
    INDEX idx_status (status),
    INDEX idx_template_status (template_type, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create observations table: %w", err)
	}
	return nil
}

// ---- resolution helpers ----

type offeringInfo struct {
	ID                int64
	Name              string
	Code              string
	Teacher           string
	TeachingClassName string
}

func (s *Store) offeringInfo(ctx context.Context, id int64) (offeringInfo, error) {
	var o offeringInfo
	var code sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT ofr.id, c.name, c.code, ofr.teacher, tc.name
		FROM course_offerings ofr
		JOIN course_catalog c ON c.id = ofr.catalog_id
		JOIN teaching_classes tc ON tc.id = ofr.teaching_class_id
		WHERE ofr.id = ?`, id,
	).Scan(&o.ID, &o.Name, &code, &o.Teacher, &o.TeachingClassName)
	if errors.Is(err, sql.ErrNoRows) {
		return offeringInfo{}, ErrCourseNotFound
	}
	if err != nil {
		return offeringInfo{}, fmt.Errorf("load course offering: %w", err)
	}
	o.Code = code.String
	return o, nil
}

type sessionInfo struct {
	ID          int64
	OfferingID  int64
	ClassroomID int64
	Date        string
	PeriodStart int
	PeriodEnd   int
}

func (s *Store) sessionInfo(ctx context.Context, id int64) (sessionInfo, error) {
	var si sessionInfo
	var date time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, offering_id, classroom_id, date, period_start, period_end FROM course_sessions WHERE id = ?`, id,
	).Scan(&si.ID, &si.OfferingID, &si.ClassroomID, &date, &si.PeriodStart, &si.PeriodEnd)
	if errors.Is(err, sql.ErrNoRows) {
		return sessionInfo{}, ErrSessionNotFound
	}
	if err != nil {
		return sessionInfo{}, fmt.Errorf("load session: %w", err)
	}
	si.Date = date.Format("2006-01-02")
	return si, nil
}

type classroomInfo struct {
	ID       int64
	Name     string
	Building string
}

func (s *Store) classroomInfo(ctx context.Context, id int64) (classroomInfo, error) {
	var ci classroomInfo
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, building FROM classrooms WHERE id = ?`, id,
	).Scan(&ci.ID, &ci.Name, &ci.Building)
	if errors.Is(err, sql.ErrNoRows) {
		return classroomInfo{}, ErrClassroomNotFound
	}
	if err != nil {
		return classroomInfo{}, fmt.Errorf("load classroom: %w", err)
	}
	return ci, nil
}

func (s *Store) displayName(ctx context.Context, id int64) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT display_name FROM users WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load user display name: %w", err)
	}
	return name, nil
}

// periodsFromRange expands a contiguous period range into an explicit list,
// matching how the legacy section list represented a session's covered periods.
func periodsFromRange(start, end int) []int {
	out := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	return out
}

// courseSnapshot is the denormalized copy of the observation's course context
// captured at save time, so a submitted record renders identically even if the
// underlying course/classroom/user is later edited.
type courseSnapshot struct {
	CourseID          int64  `json:"courseId"`
	CourseName        string `json:"courseName"`
	CourseCode        string `json:"courseCode"`
	Teacher           string `json:"teacher"`
	TeachingClassName string `json:"teachingClassName"`
	OccurrenceID      *int64 `json:"occurrenceId"`
	ObserveDate       string `json:"observeDate"`
	Sections          []int  `json:"sections"`
	ClassroomID       *int64 `json:"classroomId"`
	ClassroomName     string `json:"classroomName"`
	ObserverID        int64  `json:"observerId"`
	ObserverName      string `json:"observerName"`
}

func buildSnapshot(sn courseSnapshot) string {
	b, err := json.Marshal(sn)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// prepared carries the resolved, normalized values ready for INSERT/UPDATE.
type prepared struct {
	templateType string
	occurrenceID any // nil or int64
	courseID     int64
	classroomID  any // nil or int64
	observeDate  string
	sections     []int
	sectionsKey  any // nil or string
	scores       string
	totalScore   any // nil or float64
	content      string
	formData     string
	snapshot     string
	remark       string
}

// prepare resolves the foreign keys, normalizes sections/date, derives the
// summary columns and builds the course snapshot. observerID/observerName are
// the immutable observer (the actor, for create/update).
func (s *Store) prepare(ctx context.Context, in ObservationInput, observerID int64, observerName string) (prepared, error) {
	var p prepared
	p.templateType = strings.TrimSpace(in.TemplateType)

	if in.CourseID <= 0 {
		return p, errors.New("courseId is required")
	}
	offering, err := s.offeringInfo(ctx, in.CourseID)
	if err != nil {
		return p, err
	}
	p.courseID = offering.ID

	var session *sessionInfo
	var occurrenceID *int64
	if in.OccurrenceID != nil {
		si, err := s.sessionInfo(ctx, *in.OccurrenceID)
		if err != nil {
			return p, err
		}
		if si.OfferingID != offering.ID {
			return p, ErrSessionMismatch
		}
		session = &si
		id := si.ID
		occurrenceID = &id
		p.occurrenceID = id
	} else {
		p.occurrenceID = nil
	}

	observeDate := strings.TrimSpace(in.ObserveDate)
	if observeDate == "" && session != nil {
		observeDate = session.Date
	}
	if observeDate == "" {
		return p, errors.New("observeDate is required")
	}
	if _, err := time.Parse("2006-01-02", observeDate); err != nil {
		return p, errors.New("observeDate must be YYYY-MM-DD")
	}
	p.observeDate = observeDate

	var sections []int
	if in.Sections != nil || session == nil {
		sections, err = NormalizeSections(in.Sections)
		if err != nil {
			return p, err
		}
	} else {
		sections = periodsFromRange(session.PeriodStart, session.PeriodEnd)
	}
	p.sections = sections

	var classroomID *int64
	var classroomName string
	if in.ClassroomID != nil {
		ci, err := s.classroomInfo(ctx, *in.ClassroomID)
		if err != nil {
			return p, err
		}
		if session != nil && ci.ID != session.ClassroomID {
			return p, ErrClassroomMismatch
		}
		id := ci.ID
		classroomID = &id
		classroomName = formatClassroom(ci.Building, ci.Name)
		p.classroomID = id
	} else if session != nil {
		id := session.ClassroomID
		classroomID = &id
		p.classroomID = id
		// Resolve the name for the snapshot.
		if ci, err := s.classroomInfo(ctx, id); err == nil {
			classroomName = formatClassroom(ci.Building, ci.Name)
		}
	} else {
		p.classroomID = nil
	}

	if session != nil {
		p.sectionsKey = nil
	} else {
		p.sectionsKey = BuildSectionsKey(sections)
	}

	p.formData = rawOrDefault(in.FormData, "{}")
	scores, totalScore, content, remark := deriveSummary(in.FormData)
	p.scores = scores
	p.totalScore = totalScore
	p.content = content
	p.remark = remark

	p.snapshot = buildSnapshot(courseSnapshot{
		CourseID:          offering.ID,
		CourseName:        offering.Name,
		CourseCode:        offering.Code,
		Teacher:           offering.Teacher,
		TeachingClassName: offering.TeachingClassName,
		OccurrenceID:      occurrenceID,
		ObserveDate:       observeDate,
		Sections:          sections,
		ClassroomID:       classroomID,
		ClassroomName:     classroomName,
		ObserverID:        observerID,
		ObserverName:      observerName,
	})
	return p, nil
}

// Create inserts a draft observation owned by observer.
func (s *Store) Create(ctx context.Context, in ObservationInput, observerID int64, observerName string) (ObservationView, error) {
	p, err := s.prepare(ctx, in, observerID, observerName)
	if err != nil {
		return ObservationView{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO observations
		 (template_type, occurrence_id, course_id, classroom_id, observer_id, observe_date,
		  sections, sections_key, status, scores, total_score, content, form_data, course_snapshot, is_anonymous, remark)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.templateType, p.occurrenceID, p.courseID, p.classroomID, observerID, p.observeDate,
		marshalSections(p.sections), p.sectionsKey, StatusDraft, p.scores, p.totalScore, p.content, p.formData, p.snapshot, in.IsAnonymous, p.remark,
	)
	if err != nil {
		if dbutil.IsDuplicateEntry(err) {
			return ObservationView{}, ErrDuplicate
		}
		return ObservationView{}, fmt.Errorf("create observation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ObservationView{}, fmt.Errorf("create observation last insert id: %w", err)
	}
	return s.Get(ctx, id)
}

// Update replaces a draft observation's mutable fields. The observer is
// immutable; only the owning observer (actorID) may edit.
func (s *Store) Update(ctx context.Context, id int64, in ObservationInput, actorID int64, actorName string) (ObservationView, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return ObservationView{}, err
	}
	if existing.ObserverID != actorID {
		return ObservationView{}, ErrForbidden
	}
	if existing.Status != StatusDraft {
		return ObservationView{}, ErrSubmitted
	}

	p, err := s.prepare(ctx, in, existing.ObserverID, actorName)
	if err != nil {
		return ObservationView{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE observations
		 SET template_type = ?, occurrence_id = ?, course_id = ?, classroom_id = ?, observe_date = ?,
		     sections = ?, sections_key = ?, scores = ?, total_score = ?, content = ?, form_data = ?,
		     course_snapshot = ?, is_anonymous = ?, remark = ?
		 WHERE id = ? AND status = ?`,
		p.templateType, p.occurrenceID, p.courseID, p.classroomID, p.observeDate,
		marshalSections(p.sections), p.sectionsKey, p.scores, p.totalScore, p.content, p.formData,
		p.snapshot, in.IsAnonymous, p.remark, id, StatusDraft,
	)
	if err != nil {
		if dbutil.IsDuplicateEntry(err) {
			return ObservationView{}, ErrDuplicate
		}
		return ObservationView{}, fmt.Errorf("update observation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ObservationView{}, fmt.Errorf("update observation rows affected: %w", err)
	}
	if n == 0 {
		// Either a concurrent submit flipped the status, or the update was a
		// no-op (identical values, so MySQL reports 0 changed rows). Re-read to
		// distinguish the two.
		v, err := s.Get(ctx, id)
		if err != nil {
			return ObservationView{}, err
		}
		if v.Status != StatusDraft {
			return ObservationView{}, ErrSubmitted
		}
		return v, nil
	}
	return s.Get(ctx, id)
}

// Submit transitions a draft observation to submitted. It is idempotent: an
// already-submitted observation is returned unchanged.
func (s *Store) Submit(ctx context.Context, id, actorID int64) (ObservationView, error) {
	v, err := s.Get(ctx, id)
	if err != nil {
		return ObservationView{}, err
	}
	if v.ObserverID != actorID {
		return ObservationView{}, ErrForbidden
	}
	if v.Status == StatusSubmitted {
		return v, nil
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE observations SET status = ? WHERE id = ? AND status = ?`,
		StatusSubmitted, id, StatusDraft,
	); err != nil {
		return ObservationView{}, fmt.Errorf("submit observation: %w", err)
	}
	return s.Get(ctx, id)
}

// Delete removes a draft observation owned by actorID. Submitted observations
// cannot be deleted.
func (s *Store) Delete(ctx context.Context, id, actorID int64) error {
	v, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if v.ObserverID != actorID {
		return ErrForbidden
	}
	if v.Status != StatusDraft {
		return ErrSubmitted
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM observations WHERE id = ? AND status = ?`, id, StatusDraft)
	if err != nil {
		return fmt.Errorf("delete observation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete observation rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkExported stamps the observation's exported_at timestamp.
func (s *Store) MarkExported(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE observations SET exported_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("mark observation exported: %w", err)
	}
	return nil
}

// ---- queries ----

const obsColumns = `ob.id, ob.template_type, ob.occurrence_id, ob.course_id, ob.classroom_id, ob.observer_id,
	ob.observe_date, ob.sections, ob.status, ob.scores, ob.total_score, ob.content, ob.form_data,
	ob.course_snapshot, ob.is_anonymous, ob.remark, ob.exported_at, ob.created_at, ob.updated_at`

const obsJoin = `FROM observations ob
	JOIN course_offerings ofr ON ofr.id = ob.course_id
	JOIN course_catalog c ON c.id = ofr.catalog_id
	JOIN teaching_classes tc ON tc.id = ofr.teaching_class_id
	LEFT JOIN classrooms cr ON cr.id = ob.classroom_id
	JOIN users u ON u.id = ob.observer_id`

func buildWhere(f Filter, q string, observerID int64, admin bool) (string, []any) {
	where := ` WHERE 1=1`
	var args []any
	if !admin {
		where += ` AND ob.observer_id = ?`
		args = append(args, observerID)
	}
	if f.Status != "" {
		where += ` AND ob.status = ?`
		args = append(args, f.Status)
	}
	if f.TemplateType != "" {
		where += ` AND ob.template_type = ?`
		args = append(args, f.TemplateType)
	}
	if f.CourseID > 0 {
		where += ` AND ob.course_id = ?`
		args = append(args, f.CourseID)
	}
	if f.From != "" {
		where += ` AND ob.observe_date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND ob.observe_date <= ?`
		args = append(args, f.To)
	}
	if q != "" {
		where += ` AND (c.name LIKE ? OR c.code LIKE ? OR ofr.teacher LIKE ? OR tc.name LIKE ? OR u.display_name LIKE ? OR cr.name LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat, pat, pat, pat, pat)
	}
	return where, args
}

func (s *Store) Get(ctx context.Context, id int64) (ObservationView, error) {
	q := `SELECT ` + obsColumns + `, c.name, c.code, ofr.teacher, tc.name, cr.name, u.display_name` +
		obsJoin + ` WHERE ob.id = ?`
	v, err := scanObservationView(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ObservationView{}, ErrNotFound
	}
	if err != nil {
		return ObservationView{}, fmt.Errorf("get observation: %w", err)
	}
	return v, nil
}

func (s *Store) Page(ctx context.Context, f Filter, q string, observerID int64, admin bool, p dbutil.Pagination) ([]ObservationView, int64, error) {
	where, args := buildWhere(f, q, observerID, admin)
	query, queryArgs := p.AppendLimit(
		`SELECT `+obsColumns+`, c.name, c.code, ofr.teacher, tc.name, cr.name, u.display_name`+
			obsJoin+where+` ORDER BY ob.observe_date DESC, ob.id DESC`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page observations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	list := []ObservationView{}
	for rows.Next() {
		v, err := scanObservationView(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan observation: %w", err)
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total, err := dbutil.CountRows(ctx, s.db, obsJoin+where, args)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func scanObservationView(sc interface{ Scan(dest ...any) error }) (ObservationView, error) {
	var v ObservationView
	var observeDate time.Time
	var exportedAt sql.NullTime
	var occurrenceID, classroomID sql.NullInt64
	var totalScore sql.NullFloat64
	var sections, scores, formData, snapshot string
	var classroomName sql.NullString

	err := sc.Scan(
		&v.ID, &v.TemplateType, &occurrenceID, &v.CourseID, &classroomID, &v.ObserverID,
		&observeDate, &sections, &v.Status, &scores, &totalScore, &v.Content, &formData,
		&snapshot, &v.IsAnonymous, &v.Remark, &exportedAt, &v.CreatedAt, &v.UpdatedAt,
		&v.CourseName, &v.CourseCode, &v.Teacher, &v.TeachingClassName, &classroomName, &v.ObserverName,
	)
	if err != nil {
		return v, err
	}

	v.ObserveDate = observeDate.Format("2006-01-02")
	if occurrenceID.Valid {
		id := occurrenceID.Int64
		v.OccurrenceID = &id
	}
	if classroomID.Valid {
		id := classroomID.Int64
		v.ClassroomID = &id
	}
	if totalScore.Valid {
		v.TotalScore = &totalScore.Float64
	}
	if exportedAt.Valid {
		t := exportedAt.Time
		v.ExportedAt = &t
	}
	v.ClassroomName = classroomName.String
	v.Sections = decodeSections(sections)
	v.Scores = jsonRaw(scores, "{}")
	v.FormData = jsonRaw(formData, "{}")
	v.CourseSnapshot = jsonRaw(snapshot, "{}")
	return v, nil
}

// ---- small JSON helpers ----

func marshalSections(sections []int) string {
	b, err := json.Marshal(sections)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeSections(raw string) []int {
	if raw == "" || raw == "null" {
		return []int{}
	}
	var out []int
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []int{}
	}
	if out == nil {
		return []int{}
	}
	return out
}

// jsonRaw converts stored JSON text to a json.RawMessage, defaulting to dflt
// for empty/null values so responses always carry valid JSON objects.
func jsonRaw(raw, dflt string) json.RawMessage {
	if raw == "" || raw == "null" {
		return json.RawMessage(dflt)
	}
	return json.RawMessage(raw)
}

func rawOrDefault(v json.RawMessage, dflt string) string {
	if len(v) == 0 || string(v) == "null" {
		return dflt
	}
	return string(v)
}

// deriveSummary extracts the summary columns from the opaque form_data using
// the observation module's fixed structural keys.
func deriveSummary(formData json.RawMessage) (scores string, totalScore any, content, remark string) {
	scores = "{}"
	if len(formData) == 0 {
		return scores, nil, "", ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(formData, &m); err != nil {
		return scores, nil, "", ""
	}
	if v, ok := m["indicatorScores"]; ok {
		scores = rawOrDefault(v, "{}")
	}
	if v, ok := m["totalScore"]; ok {
		var f float64
		if err := json.Unmarshal(v, &f); err == nil {
			totalScore = f
		}
	}
	if v, ok := m["contentOutline"]; ok {
		_ = json.Unmarshal(v, &content)
	}
	if v, ok := m["comments"]; ok {
		var comments map[string]string
		if err := json.Unmarshal(v, &comments); err == nil {
			remark = comments["other"]
		}
	}
	return scores, totalScore, content, remark
}

func formatClassroom(building, name string) string {
	parts := make([]string, 0, 2)
	if building != "" {
		parts = append(parts, building)
	}
	if name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, " ")
}
