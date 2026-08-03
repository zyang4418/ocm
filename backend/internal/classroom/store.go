package classroom

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrNotFound  = errors.New("classroom not found")
	ErrNameTaken = errors.New("classroom name already taken")
)

// Controlled vocabulary for classroom type and status. Values are stored in
// English; the frontend maps them to Chinese labels.
const (
	TypeStandard    = "standard"
	TypeMultimedia  = "multimedia"
	TypeComputer    = "computer"
	TypeLab         = "lab"
	TypeLectureHall = "lecture_hall"

	StatusAvailable   = "available"
	StatusMaintenance = "maintenance"
	StatusDisabled    = "disabled"
)

const columns = "id, name, building, capacity, type, status, description, created_at"

// Store manages classroom records in the classrooms table.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the classrooms table. It is idempotent and safe to run on
// every startup.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS classrooms (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(64)  NOT NULL UNIQUE,
    building    VARCHAR(64)  NOT NULL DEFAULT '',
    capacity    INT          NOT NULL,
    type        VARCHAR(32)  NOT NULL DEFAULT 'standard',
    status      VARCHAR(32)  NOT NULL DEFAULT 'available',
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create classrooms table: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Classroom, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+columns+` FROM classrooms ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list classrooms: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var classrooms []Classroom
	for rows.Next() {
		var c Classroom
		if err := rows.Scan(&c.ID, &c.Name, &c.Building, &c.Capacity, &c.Type, &c.Status, &c.Description, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan classroom: %w", err)
		}
		classrooms = append(classrooms, c)
	}
	return classrooms, rows.Err()
}

func (s *Store) GetByID(ctx context.Context, id int64) (Classroom, error) {
	var c Classroom
	err := s.db.QueryRowContext(ctx,
		`SELECT `+columns+` FROM classrooms WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Building, &c.Capacity, &c.Type, &c.Status, &c.Description, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Classroom{}, ErrNotFound
	}
	if err != nil {
		return Classroom{}, fmt.Errorf("get classroom by id: %w", err)
	}
	return c, nil
}

func (s *Store) Create(ctx context.Context, in ClassroomInput) (Classroom, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO classrooms (name, building, capacity, type, status, description) VALUES (?, ?, ?, ?, ?, ?)`,
		in.Name, in.Building, in.Capacity, in.Type, in.Status, in.Description,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return Classroom{}, ErrNameTaken
		}
		return Classroom{}, fmt.Errorf("create classroom: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Classroom{}, fmt.Errorf("create classroom last insert id: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *Store) Update(ctx context.Context, id int64, in ClassroomInput) (Classroom, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE classrooms SET name = ?, building = ?, capacity = ?, type = ?, status = ?, description = ? WHERE id = ?`,
		in.Name, in.Building, in.Capacity, in.Type, in.Status, in.Description, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return Classroom{}, ErrNameTaken
		}
		return Classroom{}, fmt.Errorf("update classroom: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Classroom{}, fmt.Errorf("update classroom rows affected: %w", err)
	}
	if n == 0 {
		return Classroom{}, ErrNotFound
	}
	return s.GetByID(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM classrooms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete classroom: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete classroom rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// isDuplicateEntry reports whether err is a MySQL 1062 unique-constraint
// violation, used to detect duplicate classroom names on insert and update.
func isDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
