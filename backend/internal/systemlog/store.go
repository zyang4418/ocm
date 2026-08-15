package systemlog

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/logging"
)

// Setting keys and retention defaults.
const (
	settingRetentionEnabled = "log_retention_enabled"
	settingRetentionDays    = "log_retention_days"

	DefaultRetentionEnabled = true
	DefaultRetentionDays    = 180
	// MaxRetentionDays is the upper bound accepted from admins (10 years).
	MaxRetentionDays = 3650
)

// Column widths; Insert truncates defensively so an over-long annotation can
// never lose the whole row.
const (
	maxActorNameLen = 128
	maxPathLen      = 255
	maxSummaryLen   = 512
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Migrate creates the audit and settings tables and seeds the retention
// defaults. Idempotent, safe to run on every startup. INSERT IGNORE seeding
// never overwrites values an admin changed.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS system_logs (
		    id          BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
		    actor_id    BIGINT       NULL,
		    actor_name  VARCHAR(128) NOT NULL DEFAULT '',
		    method      VARCHAR(8)   NOT NULL,
		    path        VARCHAR(255) NOT NULL,
		    status_code INT          NOT NULL,
		    summary     VARCHAR(512) NULL,
		    client_ip   VARCHAR(45)  NOT NULL DEFAULT '',
		    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    KEY idx_system_logs_created_at (created_at),
		    KEY idx_system_logs_actor (actor_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS system_settings (
		    setting_key   VARCHAR(64)  NOT NULL PRIMARY KEY,
		    setting_value VARCHAR(255) NOT NULL,
		    updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("systemlog migrate: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO system_settings (setting_key, setting_value) VALUES (?, ?), (?, ?)`,
		settingRetentionEnabled, strconv.FormatBool(DefaultRetentionEnabled),
		settingRetentionDays, strconv.Itoa(DefaultRetentionDays)); err != nil {
		return fmt.Errorf("systemlog seed settings: %w", err)
	}
	return nil
}

// Insert writes one audit row. A failure is logged and swallowed — audit
// persistence must never break the business request that produced the event.
func (s *Store) Insert(ctx context.Context, e Entry) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO system_logs (actor_id, actor_name, method, path, status_code, summary, client_ip)
		 VALUES (NULLIF(?, 0), ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		e.ActorID, truncate(e.ActorName, maxActorNameLen), e.Method, truncate(e.Path, maxPathLen),
		e.StatusCode, truncate(e.Summary, maxSummaryLen), e.ClientIP)
	if err != nil {
		logging.L.Error("systemlog: insert failed", "err", err, "path", e.Path)
	}
}

// Record is Insert under an explicit name for callers outside the audited
// chain (auth login, async importer completion).
func (s *Store) Record(ctx context.Context, e Entry) { s.Insert(ctx, e) }

// PageLogs returns one page of audit rows, newest first, optionally filtered
// by day range and a contains-search over actor/summary/path.
func (s *Store) PageLogs(ctx context.Context, f LogFilter, q string, p dbutil.Pagination) ([]LogView, int64, error) {
	where := ` FROM system_logs WHERE 1=1`
	var args []any
	if f.From != "" {
		where += ` AND created_at >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND created_at < DATE_ADD(?, INTERVAL 1 DAY)`
		args = append(args, f.To)
	}
	if q != "" {
		where += ` AND (actor_name LIKE ? OR summary LIKE ? OR path LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat, pat)
	}
	// COALESCE(summary, ''): rows without an annotation store NULL, and
	// go-sql-driver refuses to scan NULL into a plain string.
	query, queryArgs := p.AppendLimit(
		`SELECT id, actor_id, actor_name, method, path, status_code, COALESCE(summary, ''), client_ip, created_at`+
			where+` ORDER BY created_at DESC, id DESC`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page logs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := []LogView{}
	for rows.Next() {
		var v LogView
		if err := rows.Scan(&v.ID, &v.ActorID, &v.ActorName, &v.Method, &v.Path,
			&v.StatusCode, &v.Summary, &v.ClientIP, &v.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan log: %w", err)
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate logs: %w", err)
	}
	total, err := dbutil.CountRows(ctx, s.db, where, args)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetSettings reads the retention policy; missing or invalid keys fall back to
// the defaults.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT setting_key, setting_value FROM system_settings`)
	if err != nil {
		return Settings{}, fmt.Errorf("load settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := Settings{RetentionEnabled: DefaultRetentionEnabled, RetentionDays: DefaultRetentionDays}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Settings{}, fmt.Errorf("scan setting: %w", err)
		}
		switch k {
		case settingRetentionEnabled:
			out.RetentionEnabled = v != "false" && v != "0"
		case settingRetentionDays:
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				out.RetentionDays = n
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Settings{}, fmt.Errorf("iterate settings: %w", err)
	}
	return out, nil
}

// UpdateSettings writes both retention keys, overwriting previous values.
func (s *Store) UpdateSettings(ctx context.Context, in Settings) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO system_settings (setting_key, setting_value) VALUES (?, ?), (?, ?)
		 ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
		settingRetentionEnabled, strconv.FormatBool(in.RetentionEnabled),
		settingRetentionDays, strconv.Itoa(in.RetentionDays)); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// PurgeExpired deletes rows older than the configured retention window and
// returns the deleted count. A no-op when retention is disabled.
func (s *Store) PurgeExpired(ctx context.Context) (int64, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return 0, err
	}
	if !settings.RetentionEnabled {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM system_logs WHERE created_at < NOW() - INTERVAL ? DAY`, settings.RetentionDays)
	if err != nil {
		return 0, fmt.Errorf("purge logs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// RunRetentionLoop purges once and then every interval until ctx is done.
// Started as a background goroutine from main (importer RecoverStale
// precedent). Recording is never disabled by settings — retention only
// controls deletion.
func (s *Store) RunRetentionLoop(ctx context.Context, interval time.Duration) {
	purge := func() {
		n, err := s.PurgeExpired(ctx)
		if err != nil {
			logging.L.Error("systemlog: retention purge failed", "err", err)
			return
		}
		if n > 0 {
			logging.L.Info("systemlog: retention purge deleted rows", "count", n)
		}
	}
	purge()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			purge()
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
