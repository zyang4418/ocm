package classroom

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"ocm-backend/internal/dbutil"
)

var (
	ErrNotFound  = errors.New("classroom not found")
	ErrNameTaken = errors.New("classroom name already taken")
)

// Controlled vocabulary for classroom type and status. Values are stored in
// English; the frontend maps them to Chinese labels. The extended types
// (stadium/drawing/language/studio/special) cover the 教务处 classroom-type
// values that the aggregated schedule import maps to these keys.
const (
	TypeStandard    = "standard"
	TypeMultimedia  = "multimedia"
	TypeComputer    = "computer"
	TypeLab         = "lab"
	TypeLectureHall = "lecture_hall"
	TypeStadium     = "stadium"  // 体育场
	TypeDrawing     = "drawing"  // 制图教室
	TypeLanguage    = "language" // 听力教室
	TypeStudio      = "studio"   // 画室
	TypeSpecial     = "special"  // 专用教室

	StatusAvailable   = "available"
	StatusMaintenance = "maintenance"
	StatusDisabled    = "disabled"
)

const columns = "id, name, building, capacity, type, floor, campus, status, description, created_at"

// Store manages classroom records in the classrooms table.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the classrooms table. It is idempotent and safe to run on
// every startup. The floor/campus columns are added via idempotent ALTERs
// (ignoring MySQL 1060 duplicate-column) so pre-existing tables are upgraded
// in place -- these columns back the 教务处 楼层/校区 fields.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS classrooms (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(64)  NOT NULL UNIQUE,
    building    VARCHAR(64)  NOT NULL DEFAULT '',
    capacity    INT          NOT NULL,
    type        VARCHAR(32)  NOT NULL DEFAULT 'standard',
    floor       VARCHAR(16)  NOT NULL DEFAULT '',
    campus      VARCHAR(32)  NOT NULL DEFAULT '',
    status      VARCHAR(32)  NOT NULL DEFAULT 'available',
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create classrooms table: %w", err)
	}
	// Upgrade pre-existing tables: add floor/campus if absent. MySQL has no
	// "ADD COLUMN IF NOT EXISTS", so ignore the duplicate-column error (1060).
	for _, col := range []struct{ name, def string }{
		{"floor", "VARCHAR(16) NOT NULL DEFAULT '' AFTER type"},
		{"campus", "VARCHAR(32) NOT NULL DEFAULT '' AFTER floor"},
	} {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE classrooms ADD COLUMN `+col.name+` `+col.def); err != nil {
			var mysqlErr *mysql.MySQLError
			if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
				return fmt.Errorf("add classrooms column %s: %w", col.name, err)
			}
		}
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
		if err := rows.Scan(&c.ID, &c.Name, &c.Building, &c.Capacity, &c.Type, &c.Floor, &c.Campus, &c.Status, &c.Description, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan classroom: %w", err)
		}
		classrooms = append(classrooms, c)
	}
	return classrooms, rows.Err()
}

// PageClassrooms returns one page of classrooms matching q (fuzzy contains on
// name and building) plus the total matching count across all pages. A zero
// Pagination means no limit (full range).
func (s *Store) PageClassrooms(ctx context.Context, q string, p dbutil.Pagination) ([]Classroom, int64, error) {
	where := ` WHERE 1=1`
	var args []any
	if q != "" {
		where += ` AND (name LIKE ? OR building LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat)
	}
	query, queryArgs := p.AppendLimit(
		`SELECT `+columns+` FROM classrooms`+where+` ORDER BY id`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page classrooms: %w", err)
	}
	defer func() { _ = rows.Close() }()

	classrooms := []Classroom{}
	for rows.Next() {
		var c Classroom
		if err := rows.Scan(&c.ID, &c.Name, &c.Building, &c.Capacity, &c.Type, &c.Floor, &c.Campus, &c.Status, &c.Description, &c.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan classroom: %w", err)
		}
		classrooms = append(classrooms, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total, err := dbutil.CountRows(ctx, s.db, `FROM classrooms`+where, args)
	if err != nil {
		return nil, 0, err
	}
	return classrooms, total, nil
}

func (s *Store) GetByID(ctx context.Context, id int64) (Classroom, error) {
	var c Classroom
	err := s.db.QueryRowContext(ctx,
		`SELECT `+columns+` FROM classrooms WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Building, &c.Capacity, &c.Type, &c.Floor, &c.Campus, &c.Status, &c.Description, &c.CreatedAt)
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
		`INSERT INTO classrooms (name, building, capacity, type, floor, campus, status, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Name, in.Building, in.Capacity, in.Type, in.Floor, in.Campus, in.Status, in.Description,
	)
	if err != nil {
		if dbutil.IsDuplicateEntry(err) {
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
	_, err := s.db.ExecContext(ctx,
		`UPDATE classrooms SET name = ?, building = ?, capacity = ?, type = ?, floor = ?, campus = ?, status = ?, description = ? WHERE id = ?`,
		in.Name, in.Building, in.Capacity, in.Type, in.Floor, in.Campus, in.Status, in.Description, id,
	)
	if err != nil {
		if dbutil.IsDuplicateEntry(err) {
			return Classroom{}, ErrNameTaken
		}
		return Classroom{}, fmt.Errorf("update classroom: %w", err)
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
