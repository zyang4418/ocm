package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Migrate creates the single-row settings table and seeds the default row.
// Idempotent, safe to run on every startup. INSERT IGNORE seeding never
// overwrites values an admin changed.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS mail_settings (
		    id           INT UNSIGNED NOT NULL PRIMARY KEY,
		    enabled      TINYINT(1)   NOT NULL DEFAULT 0,
		    host         VARCHAR(255) NOT NULL DEFAULT '',
		    port         INT          NOT NULL DEFAULT 465,
		    username     VARCHAR(255) NOT NULL DEFAULT '',
		    password     VARCHAR(255) NOT NULL DEFAULT '',
		    from_name    VARCHAR(255) NOT NULL DEFAULT '',
		    from_address VARCHAR(255) NOT NULL DEFAULT '',
		    encryption   VARCHAR(16)  NOT NULL DEFAULT 'ssl',
		    updated_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("mail migrate: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO mail_settings (id) VALUES (1)`); err != nil {
		return fmt.Errorf("mail seed settings: %w", err)
	}
	return nil
}

// Get reads the settings row. A missing row (hand-edited database) falls back
// to the defaults so the API always has something coherent to return.
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var v Settings
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled, host, port, username, password, from_name, from_address, encryption
		 FROM mail_settings WHERE id = 1`).
		Scan(&v.Enabled, &v.Host, &v.Port, &v.Username, &v.Password, &v.FromName,
			&v.FromAddress, &v.Encryption)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{Port: DefaultPort, Encryption: EncryptionSSL}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("load mail settings: %w", err)
	}
	return v, nil
}

// Update overwrites the whole settings row. The handler resolves keep-vs-
// replace for the password before calling, so this stays one unconditional
// UPDATE.
func (s *Store) Update(ctx context.Context, in Settings) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE mail_settings
		 SET enabled = ?, host = ?, port = ?, username = ?, password = ?,
		     from_name = ?, from_address = ?, encryption = ?
		 WHERE id = 1`,
		in.Enabled, in.Host, in.Port, in.Username, in.Password,
		in.FromName, in.FromAddress, in.Encryption); err != nil {
		return fmt.Errorf("update mail settings: %w", err)
	}
	return nil
}
