package storage

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
		`CREATE TABLE IF NOT EXISTS storage_settings (
		    id              INT UNSIGNED NOT NULL PRIMARY KEY,
		    enabled         TINYINT(1)   NOT NULL DEFAULT 0,
		    endpoint        VARCHAR(255) NOT NULL DEFAULT '',
		    region          VARCHAR(64)  NOT NULL DEFAULT '',
		    bucket          VARCHAR(255) NOT NULL DEFAULT '',
		    access_key      VARCHAR(255) NOT NULL DEFAULT '',
		    secret_key      VARCHAR(255) NOT NULL DEFAULT '',
		    use_ssl         TINYINT(1)   NOT NULL DEFAULT 1,
		    use_path_style  TINYINT(1)   NOT NULL DEFAULT 1,
		    public_base_url VARCHAR(255) NOT NULL DEFAULT '',
		    updated_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("storage migrate: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO storage_settings (id) VALUES (1)`); err != nil {
		return fmt.Errorf("storage seed settings: %w", err)
	}
	return nil
}

// Get reads the settings row. A missing row (hand-edited database) falls back
// to the defaults so the API always has something coherent to return.
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var v Settings
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled, endpoint, region, bucket, access_key, secret_key,
		        use_ssl, use_path_style, public_base_url
		 FROM storage_settings WHERE id = 1`).
		Scan(&v.Enabled, &v.Endpoint, &v.Region, &v.Bucket, &v.AccessKey, &v.SecretKey,
			&v.UseSSL, &v.UsePathStyle, &v.PublicBaseURL)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{UseSSL: true, UsePathStyle: true}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("load storage settings: %w", err)
	}
	return v, nil
}

// Update overwrites the whole settings row. The handler resolves keep-vs-
// replace for the secret key before calling, so this stays one unconditional
// UPDATE.
func (s *Store) Update(ctx context.Context, in Settings) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE storage_settings
		 SET enabled = ?, endpoint = ?, region = ?, bucket = ?, access_key = ?,
		     secret_key = ?, use_ssl = ?, use_path_style = ?, public_base_url = ?
		 WHERE id = 1`,
		in.Enabled, in.Endpoint, in.Region, in.Bucket, in.AccessKey, in.SecretKey,
		in.UseSSL, in.UsePathStyle, in.PublicBaseURL); err != nil {
		return fmt.Errorf("update storage settings: %w", err)
	}
	return nil
}
