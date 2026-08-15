package ai

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
		`CREATE TABLE IF NOT EXISTS ai_settings (
		    id         INT UNSIGNED NOT NULL PRIMARY KEY,
		    enabled    TINYINT(1)   NOT NULL DEFAULT 0,
		    base_url   VARCHAR(255) NOT NULL DEFAULT '',
		    api_key    VARCHAR(255) NOT NULL DEFAULT '',
		    model      VARCHAR(128) NOT NULL DEFAULT '',
		    updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("ai migrate: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO ai_settings (id) VALUES (1)`); err != nil {
		return fmt.Errorf("ai seed settings: %w", err)
	}
	return nil
}

// Get reads the settings row. A missing row (hand-edited database) falls back
// to the zero-value settings so the API always has something coherent to
// return (disabled, empty fields).
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var v Settings
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled, base_url, api_key, model FROM ai_settings WHERE id = 1`).
		Scan(&v.Enabled, &v.BaseURL, &v.APIKey, &v.Model)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("load ai settings: %w", err)
	}
	return v, nil
}

// Update overwrites the whole settings row. The handler resolves keep-vs-
// replace for the API key before calling, so this stays one unconditional
// UPDATE.
func (s *Store) Update(ctx context.Context, in Settings) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE ai_settings
		 SET enabled = ?, base_url = ?, api_key = ?, model = ?
		 WHERE id = 1`,
		in.Enabled, in.BaseURL, in.APIKey, in.Model); err != nil {
		return fmt.Errorf("update ai settings: %w", err)
	}
	return nil
}
