package course

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrCatalogNotFound       = errors.New("course not found")
	ErrOfferingNotFound      = errors.New("course offering not found")
	ErrSessionNotFound       = errors.New("session not found")
	ErrNameTaken             = errors.New("course name already taken")
	ErrCodeTaken             = errors.New("course code already taken")
	ErrOfferingTaken         = errors.New("course offering already exists")
	ErrTeachingClassNotFound = errors.New("teaching class not found")
	ErrInUse                 = errors.New("record is in use and cannot be deleted")
	ErrClassroomConflict     = errors.New("classroom already booked for this date and period")
)

// Store manages the course catalog, offerings and sessions tables.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// nullableCode returns nil for an empty code so the column stores NULL (not '').
// Multiple NULLs are distinct under UNIQUE(code); multiple '' would collide.
// Callers that read the value back get "" either way (NULL scans to the zero
// string), so the empty-means-uncoded convention is preserved on read.
func nullableCode(code string) interface{} {
	if code == "" {
		return nil
	}
	return code
}

// Migrate creates the catalog, offering and session tables. It is idempotent.
// Idempotent ALTERs (ignoring MySQL 1060 duplicate-column / 1061 duplicate-key)
// upgrade pre-existing tables in place: the catalog gains academic-attribute
// columns + a UNIQUE(code) index, and offerings gain 教务处 section-metadata
// columns. See backend/internal/course/README.md for the column rationale.
func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS course_catalog (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(128) NOT NULL UNIQUE,
    code        VARCHAR(64)  NULL DEFAULT NULL,
    credits     DECIMAL(4,1) NOT NULL DEFAULT 0,
    total_hours INT          NOT NULL DEFAULT 0,
    category    VARCHAR(32)  NOT NULL DEFAULT '',
    exam_type   VARCHAR(16)  NOT NULL DEFAULT '',
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS course_offerings (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY,
    catalog_id        BIGINT       NOT NULL,
    teaching_class_id BIGINT       NOT NULL,
    teacher           VARCHAR(64)  NOT NULL DEFAULT '',
    course_seq        VARCHAR(32)  NOT NULL DEFAULT '',
    teacher_id        VARCHAR(64)  NOT NULL DEFAULT '',
    teacher_title     VARCHAR(32)  NOT NULL DEFAULT '',
    college           VARCHAR(64)  NOT NULL DEFAULT '',
    max_students      INT          NOT NULL DEFAULT 0,
    requirement       VARCHAR(16)  NOT NULL DEFAULT '',
    weekly_hours      INT          NOT NULL DEFAULT 0,
    semester          VARCHAR(32)  NOT NULL,
    note              VARCHAR(255) NOT NULL DEFAULT '',
    created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (catalog_id, teaching_class_id, semester)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS course_sessions (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    offering_id  BIGINT NOT NULL,
    classroom_id BIGINT NOT NULL,
    date         DATE NOT NULL,
    period_index INT NOT NULL,
    note         VARCHAR(255) NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (classroom_id, date, period_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create course table: %w", err)
		}
	}
	// Upgrade pre-existing catalog table: add attribute columns + UNIQUE(code).
	catalogCols := []struct{ name, def string }{
		{"credits", "DECIMAL(4,1) NOT NULL DEFAULT 0 AFTER code"},
		{"total_hours", "INT NOT NULL DEFAULT 0 AFTER credits"},
		{"category", "VARCHAR(32) NOT NULL DEFAULT '' AFTER total_hours"},
		{"exam_type", "VARCHAR(16) NOT NULL DEFAULT '' AFTER category"},
	}
	for _, col := range catalogCols {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE course_catalog ADD COLUMN `+col.name+` `+col.def); err != nil {
			var mysqlErr *mysql.MySQLError
			if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
				return fmt.Errorf("add catalog column %s: %w", col.name, err)
			}
		}
	}
	// UNIQUE(code): code is NULL-able so courses without a code (NULL) do not
	// collide — MySQL treats multiple NULLs as distinct in a UNIQUE index, while
	// empty strings '' would all compare equal and violate the index. Upgrade
	// path: widen the column to NULL DEFAULT NULL, migrate legacy '' rows to
	// NULL, then add the unique key (idempotent; ignore 1061 duplicate-key-name).
	// A data-level duplicate (1062) is intentionally NOT ignored: it signals two
	// distinct courses wrongly sharing a code, which the caller must resolve.
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE course_catalog MODIFY COLUMN code VARCHAR(64) NULL DEFAULT NULL`); err != nil {
		return fmt.Errorf("modify catalog code column: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE course_catalog SET code = NULL WHERE code = ''`); err != nil {
		return fmt.Errorf("null out empty catalog codes: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE course_catalog ADD UNIQUE KEY uq_catalog_code (code)`); err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1061 {
			return fmt.Errorf("add catalog unique code: %w", err)
		}
	}
	// Upgrade pre-existing offerings table: add 教务处 section-metadata columns.
	offeringCols := []struct{ name, def string }{
		{"course_seq", "VARCHAR(32) NOT NULL DEFAULT '' AFTER teacher"},
		{"teacher_id", "VARCHAR(64) NOT NULL DEFAULT '' AFTER course_seq"},
		{"teacher_title", "VARCHAR(32) NOT NULL DEFAULT '' AFTER teacher_id"},
		{"college", "VARCHAR(64) NOT NULL DEFAULT '' AFTER teacher_title"},
		{"max_students", "INT NOT NULL DEFAULT 0 AFTER college"},
		{"requirement", "VARCHAR(16) NOT NULL DEFAULT '' AFTER max_students"},
		{"weekly_hours", "INT NOT NULL DEFAULT 0 AFTER requirement"},
	}
	for _, col := range offeringCols {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE course_offerings ADD COLUMN `+col.name+` `+col.def); err != nil {
			var mysqlErr *mysql.MySQLError
			if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
				return fmt.Errorf("add offering column %s: %w", col.name, err)
			}
		}
	}
	return nil
}

// ---- Catalog ----

func (s *Store) ListCatalog(ctx context.Context) ([]CatalogCourse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, code, credits, total_hours, category, exam_type, description, created_at FROM course_catalog ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []CatalogCourse
	for rows.Next() {
		var c CatalogCourse
		if err := rows.Scan(&c.ID, &c.Name, &c.Code, &c.Credits, &c.TotalHours, &c.Category, &c.ExamType, &c.Description, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan catalog: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetCatalog(ctx context.Context, id int64) (CatalogCourse, error) {
	var c CatalogCourse
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, code, credits, total_hours, category, exam_type, description, created_at FROM course_catalog WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Code, &c.Credits, &c.TotalHours, &c.Category, &c.ExamType, &c.Description, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CatalogCourse{}, ErrCatalogNotFound
	}
	if err != nil {
		return CatalogCourse{}, fmt.Errorf("get catalog: %w", err)
	}
	return c, nil
}

func (s *Store) CreateCatalog(ctx context.Context, in CatalogInput) (CatalogCourse, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO course_catalog (name, code, credits, total_hours, category, exam_type, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.Name, nullableCode(in.Code), in.Credits, in.TotalHours, in.Category, in.ExamType, in.Description,
	)
	if err != nil {
		switch duplicateKeyName(err) {
		case "uq_catalog_code":
			return CatalogCourse{}, ErrCodeTaken
		case "name", "":
			if isDuplicateEntry(err) {
				return CatalogCourse{}, ErrNameTaken
			}
		}
		return CatalogCourse{}, fmt.Errorf("create catalog: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CatalogCourse{}, fmt.Errorf("create catalog last insert id: %w", err)
	}
	return s.GetCatalog(ctx, id)
}

func (s *Store) UpdateCatalog(ctx context.Context, id int64, in CatalogInput) (CatalogCourse, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE course_catalog SET name = ?, code = ?, credits = ?, total_hours = ?, category = ?, exam_type = ?, description = ? WHERE id = ?`,
		in.Name, nullableCode(in.Code), in.Credits, in.TotalHours, in.Category, in.ExamType, in.Description, id,
	)
	if err != nil {
		switch duplicateKeyName(err) {
		case "uq_catalog_code":
			return CatalogCourse{}, ErrCodeTaken
		case "name", "":
			if isDuplicateEntry(err) {
				return CatalogCourse{}, ErrNameTaken
			}
		}
		return CatalogCourse{}, fmt.Errorf("update catalog: %w", err)
	}
	return s.GetCatalog(ctx, id)
}

func (s *Store) DeleteCatalog(ctx context.Context, id int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM course_offerings WHERE catalog_id = ?`, id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count offerings: %w", err)
	}
	if count > 0 {
		return ErrInUse
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM course_catalog WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete catalog: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete catalog rows affected: %w", err)
	}
	if n == 0 {
		return ErrCatalogNotFound
	}
	return nil
}

// ---- Offerings ----

func (s *Store) ListOfferings(ctx context.Context) ([]OfferingView, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT o.id, o.catalog_id, o.teaching_class_id, o.teacher, o.course_seq, o.teacher_id, o.teacher_title, o.college, o.max_students, o.requirement, o.weekly_hours, o.semester, o.note, o.created_at,
       c.name, c.code, tc.name
FROM course_offerings o
JOIN course_catalog c ON c.id = o.catalog_id
JOIN teaching_classes tc ON tc.id = o.teaching_class_id
ORDER BY o.semester DESC, tc.name, c.name`)
	if err != nil {
		return nil, fmt.Errorf("list offerings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []OfferingView
	var tcIDs []int64
	for rows.Next() {
		var v OfferingView
		if err := rows.Scan(
			&v.ID, &v.CatalogID, &v.TeachingClassID, &v.Teacher, &v.CourseSeq, &v.TeacherID, &v.TeacherTitle, &v.College, &v.MaxStudents, &v.Requirement, &v.WeeklyHours, &v.Semester, &v.Note, &v.CreatedAt,
			&v.CatalogName, &v.CatalogCode, &v.TeachingClassName,
		); err != nil {
			return nil, fmt.Errorf("scan offering: %w", err)
		}
		list = append(list, v)
		tcIDs = append(tcIDs, v.TeachingClassID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names, err := s.classNamesByTeachingClass(ctx, tcIDs)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].ClassNames = names[list[i].TeachingClassID]
		if list[i].ClassNames == nil {
			list[i].ClassNames = []string{}
		}
	}
	return list, nil
}

func (s *Store) GetOffering(ctx context.Context, id int64) (OfferingView, error) {
	var v OfferingView
	err := s.db.QueryRowContext(ctx, `
SELECT o.id, o.catalog_id, o.teaching_class_id, o.teacher, o.course_seq, o.teacher_id, o.teacher_title, o.college, o.max_students, o.requirement, o.weekly_hours, o.semester, o.note, o.created_at,
       c.name, c.code, tc.name
FROM course_offerings o
JOIN course_catalog c ON c.id = o.catalog_id
JOIN teaching_classes tc ON tc.id = o.teaching_class_id
WHERE o.id = ?`, id,
	).Scan(
		&v.ID, &v.CatalogID, &v.TeachingClassID, &v.Teacher, &v.CourseSeq, &v.TeacherID, &v.TeacherTitle, &v.College, &v.MaxStudents, &v.Requirement, &v.WeeklyHours, &v.Semester, &v.Note, &v.CreatedAt,
		&v.CatalogName, &v.CatalogCode, &v.TeachingClassName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return OfferingView{}, ErrOfferingNotFound
	}
	if err != nil {
		return OfferingView{}, fmt.Errorf("get offering: %w", err)
	}
	names, err := s.classNamesByTeachingClass(ctx, []int64{v.TeachingClassID})
	if err != nil {
		return OfferingView{}, err
	}
	v.ClassNames = names[v.TeachingClassID]
	if v.ClassNames == nil {
		v.ClassNames = []string{}
	}
	return v, nil
}

func (s *Store) CreateOffering(ctx context.Context, in OfferingInput) (OfferingView, error) {
	if err := s.teachingClassExists(ctx, in.TeachingClassID); err != nil {
		return OfferingView{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO course_offerings (catalog_id, teaching_class_id, teacher, course_seq, teacher_id, teacher_title, college, max_students, requirement, weekly_hours, semester, note) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.CatalogID, in.TeachingClassID, in.Teacher, in.CourseSeq, in.TeacherID, in.TeacherTitle, in.College, in.MaxStudents, in.Requirement, in.WeeklyHours, in.Semester, in.Note,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return OfferingView{}, ErrOfferingTaken
		}
		return OfferingView{}, fmt.Errorf("create offering: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return OfferingView{}, fmt.Errorf("create offering last insert id: %w", err)
	}
	return s.GetOffering(ctx, id)
}

func (s *Store) UpdateOffering(ctx context.Context, id int64, in OfferingInput) (OfferingView, error) {
	if err := s.teachingClassExists(ctx, in.TeachingClassID); err != nil {
		return OfferingView{}, err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE course_offerings SET catalog_id = ?, teaching_class_id = ?, teacher = ?, course_seq = ?, teacher_id = ?, teacher_title = ?, college = ?, max_students = ?, requirement = ?, weekly_hours = ?, semester = ?, note = ? WHERE id = ?`,
		in.CatalogID, in.TeachingClassID, in.Teacher, in.CourseSeq, in.TeacherID, in.TeacherTitle, in.College, in.MaxStudents, in.Requirement, in.WeeklyHours, in.Semester, in.Note, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return OfferingView{}, ErrOfferingTaken
		}
		return OfferingView{}, fmt.Errorf("update offering: %w", err)
	}
	return s.GetOffering(ctx, id)
}

// teachingClassExists returns ErrTeachingClassNotFound when no teaching class
// has the given id. Teaching classes live in the user module; this is a logical
// cross-module FK check (no Go dependency on the user package).
func (s *Store) teachingClassExists(ctx context.Context, id int64) error {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM teaching_classes WHERE id = ?`, id,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTeachingClassNotFound
	}
	if err != nil {
		return fmt.Errorf("check teaching class exists: %w", err)
	}
	return nil
}

// classNamesByTeachingClass loads the member admin class names for the given
// teaching class IDs in one query, returning a map keyed by teaching_class_id.
// Used to populate OfferingView.ClassNames / SessionView.ClassNames.
func (s *Store) classNamesByTeachingClass(ctx context.Context, ids []int64) (map[int64][]string, error) {
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
		`SELECT m.teaching_class_id, ac.name
		 FROM teaching_class_members m
		 JOIN admin_classes ac ON ac.id = m.admin_class_id
		 WHERE m.teaching_class_id IN (%s)
		 ORDER BY m.teaching_class_id, ac.grade, ac.name`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query teaching class member names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tcID int64
		var name string
		if err := rows.Scan(&tcID, &name); err != nil {
			return nil, fmt.Errorf("scan member name: %w", err)
		}
		out[tcID] = append(out[tcID], name)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOffering(ctx context.Context, id int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM course_sessions WHERE offering_id = ?`, id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count sessions: %w", err)
	}
	if count > 0 {
		return ErrInUse
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM course_offerings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete offering: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete offering rows affected: %w", err)
	}
	if n == 0 {
		return ErrOfferingNotFound
	}
	return nil
}

func isDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// duplicateKeyName inspects a MySQL 1062 (duplicate entry) error and returns the
// index name that caused the collision, e.g. "name" or "uq_catalog_code". It
// returns "" for non-duplicate errors. Callers use it to map a 1062 to the right
// sentinel (ErrNameTaken vs ErrCodeTaken) instead of guessing from the message.
func duplicateKeyName(err error) string {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
		return ""
	}
	// Message format: `Duplicate entry '<val>' for key '<key>'`.
	s := mysqlErr.Message
	i := strings.LastIndex(s, "for key '")
	if i < 0 {
		return ""
	}
	s = s[i+len("for key '"):]
	if j := strings.Index(s, "'"); j >= 0 {
		return s[:j]
	}
	return ""
}
