package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound      = errors.New("user not found")
	ErrUsernameTaken = errors.New("username already taken")
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Store manages user records in the users table (created by auth.Store.Migrate).
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, display_name, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, role, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (s *Store) GetByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, role, created_at FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

func (s *Store) Create(ctx context.Context, in CreateUserInput) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, display_name, role) VALUES (?, ?, ?, ?)`,
		in.Username, string(hash), in.DisplayName, in.Role,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return User{}, ErrUsernameTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("create user last insert id: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *Store) Update(ctx context.Context, id int64, in UpdateUserInput) (User, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET display_name = ?, role = ? WHERE id = ?`,
		in.DisplayName, in.Role, id,
	)
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *Store) UpdatePassword(ctx context.Context, id int64, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`,
		string(hash), id,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update password rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
