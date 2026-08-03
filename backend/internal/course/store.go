package course

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrCatalogNotFound   = errors.New("course not found")
	ErrOfferingNotFound  = errors.New("course offering not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrNameTaken         = errors.New("course name already taken")
	ErrOfferingTaken     = errors.New("course offering already exists")
	ErrInUse             = errors.New("record is in use and cannot be deleted")
	ErrClassroomConflict = errors.New("classroom already booked for this date and period")
)

// Store manages the course catalog, offerings and sessions tables.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the catalog, offering and session tables. It is idempotent.
func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS course_catalog (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(128) NOT NULL UNIQUE,
    code        VARCHAR(64)  NOT NULL DEFAULT '',
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS course_offerings (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    catalog_id  BIGINT       NOT NULL,
    class_name  VARCHAR(64)  NOT NULL,
    teacher     VARCHAR(64)  NOT NULL DEFAULT '',
    semester    VARCHAR(32)  NOT NULL,
    note        VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (catalog_id, class_name, semester)
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
	return nil
}

// ---- Catalog ----

func (s *Store) ListCatalog(ctx context.Context) ([]CatalogCourse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, code, description, created_at FROM course_catalog ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []CatalogCourse
	for rows.Next() {
		var c CatalogCourse
		if err := rows.Scan(&c.ID, &c.Name, &c.Code, &c.Description, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan catalog: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetCatalog(ctx context.Context, id int64) (CatalogCourse, error) {
	var c CatalogCourse
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, code, description, created_at FROM course_catalog WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Code, &c.Description, &c.CreatedAt)
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
		`INSERT INTO course_catalog (name, code, description) VALUES (?, ?, ?)`,
		in.Name, in.Code, in.Description,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return CatalogCourse{}, ErrNameTaken
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
		`UPDATE course_catalog SET name = ?, code = ?, description = ? WHERE id = ?`,
		in.Name, in.Code, in.Description, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return CatalogCourse{}, ErrNameTaken
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
SELECT o.id, o.catalog_id, o.class_name, o.teacher, o.semester, o.note, o.created_at,
       c.name, c.code
FROM course_offerings o
JOIN course_catalog c ON c.id = o.catalog_id
ORDER BY o.semester DESC, o.class_name, c.name`)
	if err != nil {
		return nil, fmt.Errorf("list offerings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []OfferingView
	for rows.Next() {
		var v OfferingView
		if err := rows.Scan(
			&v.ID, &v.CatalogID, &v.ClassName, &v.Teacher, &v.Semester, &v.Note, &v.CreatedAt,
			&v.CatalogName, &v.CatalogCode,
		); err != nil {
			return nil, fmt.Errorf("scan offering: %w", err)
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (s *Store) GetOffering(ctx context.Context, id int64) (OfferingView, error) {
	var v OfferingView
	err := s.db.QueryRowContext(ctx, `
SELECT o.id, o.catalog_id, o.class_name, o.teacher, o.semester, o.note, o.created_at,
       c.name, c.code
FROM course_offerings o
JOIN course_catalog c ON c.id = o.catalog_id
WHERE o.id = ?`, id,
	).Scan(
		&v.ID, &v.CatalogID, &v.ClassName, &v.Teacher, &v.Semester, &v.Note, &v.CreatedAt,
		&v.CatalogName, &v.CatalogCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return OfferingView{}, ErrOfferingNotFound
	}
	if err != nil {
		return OfferingView{}, fmt.Errorf("get offering: %w", err)
	}
	return v, nil
}

func (s *Store) CreateOffering(ctx context.Context, in OfferingInput) (OfferingView, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO course_offerings (catalog_id, class_name, teacher, semester, note) VALUES (?, ?, ?, ?, ?)`,
		in.CatalogID, in.ClassName, in.Teacher, in.Semester, in.Note,
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
	_, err := s.db.ExecContext(ctx,
		`UPDATE course_offerings SET catalog_id = ?, class_name = ?, teacher = ?, semester = ?, note = ? WHERE id = ?`,
		in.CatalogID, in.ClassName, in.Teacher, in.Semester, in.Note, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return OfferingView{}, ErrOfferingTaken
		}
		return OfferingView{}, fmt.Errorf("update offering: %w", err)
	}
	return s.GetOffering(ctx, id)
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
