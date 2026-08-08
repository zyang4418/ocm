package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

const defaultAdminPassword = "admin123"

// User is an account allowed to sign in to the admin console.
type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

// Store persists users in MySQL.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the users table and seeds the initial admin account.
// It is idempotent and safe to run on every startup.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    username      VARCHAR(64)  NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name  VARCHAR(128) NOT NULL,
    role          VARCHAR(32)  NOT NULL DEFAULT 'admin',
    openid        VARCHAR(64)  NULL DEFAULT NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE INDEX idx_users_openid (openid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	// Add the openid column for tables created before it existed. MySQL has no
	// "ADD COLUMN IF NOT EXISTS", so ignore the duplicate-column error (1060).
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE users ADD COLUMN openid VARCHAR(64) NULL DEFAULT NULL`,
	); err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
			return fmt.Errorf("add openid column: %w", err)
		}
	}
	// Add the unique index on openid for tables created before it existed;
	// ignore the duplicate-index error (1061). Multiple NULLs are allowed, so
	// unbound accounts coexist while each openid binds at most one account.
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE users ADD UNIQUE INDEX idx_users_openid (openid)`,
	); err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1061 {
			return fmt.Errorf("add openid index: %w", err)
		}
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		password = defaultAdminPassword
		log.Printf("auth: ADMIN_PASSWORD not set, seeding admin with default password %q -- change it before production use", defaultAdminPassword)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, display_name, role) VALUES (?, ?, ?, ?)`,
		"admin", string(hash), "系统管理员", "admin",
	); err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}
	log.Print("auth: seeded initial admin account (username: admin)")
	return nil
}

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAlreadyBound       = errors.New("account is already bound to a WeChat id")
	ErrOpenidTaken        = errors.New("WeChat id is already bound to another account")
	ErrNotBound           = errors.New("WeChat id is not bound to any account")
)

// Authenticate verifies username/password and returns the user on success.
func (s *Store) Authenticate(ctx context.Context, username, password string) (User, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, role FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &hash, &u.DisplayName, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("query user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrInvalidCredentials
	}
	return u, nil
}

// ByUsername loads a user for token validation (e.g. /api/auth/me).
func (s *Store) ByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, role FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

// GetByOpenid loads a user by their bound WeChat openid (silent re-login).
func (s *Store) GetByOpenid(ctx context.Context, openid string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, role FROM users WHERE openid = ?`,
		openid,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotBound
	}
	if err != nil {
		return User{}, fmt.Errorf("query user by openid: %w", err)
	}
	return u, nil
}

// BindOpenid links a WeChat openid to a user account. It only succeeds when the
// account has no openid yet (re-binding requires unbinding first) and the
// openid is not already linked to another account (enforced by the UNIQUE
// index, returned as ErrOpenidTaken).
func (s *Store) BindOpenid(ctx context.Context, userID int64, openid string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET openid = ? WHERE id = ? AND openid IS NULL`,
		openid, userID,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrOpenidTaken
		}
		return fmt.Errorf("bind openid: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("bind openid rows affected: %w", err)
	}
	if n == 0 {
		return ErrAlreadyBound
	}
	return nil
}

// UnbindOpenid clears the WeChat openid bound to a user (looked up by
// username), forcing the next mini-program entry to re-bind with credentials.
func (s *Store) UnbindOpenid(ctx context.Context, username string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET openid = NULL WHERE username = ?`, username,
	); err != nil {
		return fmt.Errorf("unbind openid: %w", err)
	}
	return nil
}
