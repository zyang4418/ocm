package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrJobNotFound      = errors.New("import job not found")
	ErrJobStateConflict = errors.New("import job not in expected state")
)

const jobColumns = `id, type, status, filename, payload, total_rows, succeeded_rows, failed_rows, error_report, preview, user_id, created_at, started_at, finished_at`

// Store manages import job records in the import_jobs table.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the import_jobs table. Idempotent.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS import_jobs (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    type           VARCHAR(32)  NOT NULL,
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending',
    filename       VARCHAR(255) NOT NULL DEFAULT '',
    payload        LONGTEXT     NOT NULL,
    total_rows     INT          NOT NULL DEFAULT 0,
    succeeded_rows INT          NOT NULL DEFAULT 0,
    failed_rows    INT          NOT NULL DEFAULT 0,
    error_report   LONGTEXT     NOT NULL,
    preview        LONGTEXT     NULL DEFAULT NULL,
    user_id        BIGINT       NOT NULL,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at     TIMESTAMP    NULL DEFAULT NULL,
    finished_at    TIMESTAMP    NULL DEFAULT NULL,
    INDEX idx_user (user_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create import_jobs table: %w", err)
	}
	// Add the preview column for tables created before it existed. MySQL has no
	// "ADD COLUMN IF NOT EXISTS", so ignore the duplicate-column error (1060).
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE import_jobs ADD COLUMN preview LONGTEXT NULL DEFAULT NULL AFTER error_report`,
	); err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
			return fmt.Errorf("add preview column: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateJob(ctx context.Context, typ, filename, payload string, userID int64) (Job, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO import_jobs (type, status, filename, payload, error_report, user_id) VALUES (?, 'pending', ?, ?, '', ?)`,
		typ, filename, payload, userID,
	)
	if err != nil {
		return Job{}, fmt.Errorf("create import job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Job{}, fmt.Errorf("create import job last insert id: %w", err)
	}
	return s.GetJob(ctx, id)
}

func (s *Store) GetJob(ctx context.Context, id int64) (Job, error) {
	var j Job
	var started, finished sql.NullTime
	var preview sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM import_jobs WHERE id = ?`, id,
	).Scan(
		&j.ID, &j.Type, &j.Status, &j.Filename, &j.Payload, &j.TotalRows, &j.SucceededRows, &j.FailedRows, &j.ErrorReport, &preview, &j.UserID, &j.CreatedAt, &started, &finished,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("get import job: %w", err)
	}
	if preview.Valid && preview.String != "" && preview.String != "null" {
		_ = json.Unmarshal([]byte(preview.String), &j.Rows)
	}
	if started.Valid {
		j.StartedAt = &started.Time
	}
	if finished.Valid {
		j.FinishedAt = &finished.Time
	}
	return j, nil
}

// ListJobs returns the most recent jobs (newest first), excluding the payload
// column to keep responses small.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, status, filename, total_rows, succeeded_rows, failed_rows, error_report, user_id, created_at, started_at, finished_at FROM import_jobs ORDER BY created_at DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []Job
	for rows.Next() {
		var j Job
		var started, finished sql.NullTime
		if err := rows.Scan(
			&j.ID, &j.Type, &j.Status, &j.Filename, &j.TotalRows, &j.SucceededRows, &j.FailedRows, &j.ErrorReport, &j.UserID, &j.CreatedAt, &started, &finished,
		); err != nil {
			return nil, fmt.Errorf("scan import job: %w", err)
		}
		if started.Valid {
			j.StartedAt = &started.Time
		}
		if finished.Valid {
			j.FinishedAt = &finished.Time
		}
		list = append(list, j)
	}
	return list, rows.Err()
}

// MarkProcessing transitions a job to processing and stamps started_at. It is
// conditional on fromStatus so the transition is atomic with the status
// precondition: a concurrent transition that changed the status first causes
// this to affect zero rows and return ErrJobStateConflict. (processJob passes
// pending; commitJob passes preview.)
func (s *Store) MarkProcessing(ctx context.Context, id int64, fromStatus string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = ?, started_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`,
		StatusProcessing, id, fromStatus,
	)
	if err != nil {
		return fmt.Errorf("mark import job processing: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark import job processing rows affected: %w", err)
	}
	if n == 0 {
		return ErrJobStateConflict
	}
	return nil
}

// SavePreview records the dry-run result of parsing/validation without writing
// any sessions, transitioning the job to the preview state for user review.
func (s *Store) SavePreview(ctx context.Context, id int64, r Result) error {
	errs := r.Errors
	if errs == nil {
		errs = []RowError{}
	}
	rows := r.Rows
	if rows == nil {
		rows = []map[string]any{}
	}
	errJSON, err := json.Marshal(errs)
	if err != nil {
		errJSON = []byte("[]")
	}
	rowsJSON, err := json.Marshal(rows)
	if err != nil {
		rowsJSON = []byte("[]")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = ?, total_rows = ?, succeeded_rows = ?, failed_rows = ?, error_report = ?, preview = ? WHERE id = ?`,
		StatusPreview, r.TotalRows, r.SucceededRows, r.FailedRows, string(errJSON), string(rowsJSON), id,
	); err != nil {
		return fmt.Errorf("save import preview: %w", err)
	}
	return nil
}

// Finish writes the final status, row counts, error report and finished_at,
// and clears the preview rows (no longer needed once the job is resolved).
func (s *Store) Finish(ctx context.Context, id int64, status string, r Result) error {
	errs := r.Errors
	if errs == nil {
		errs = []RowError{}
	}
	report, err := json.Marshal(errs)
	if err != nil {
		report = []byte("[]")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = ?, total_rows = ?, succeeded_rows = ?, failed_rows = ?, error_report = ?, preview = NULL, finished_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, r.TotalRows, r.SucceededRows, r.FailedRows, string(report), id,
	); err != nil {
		return fmt.Errorf("finish import job: %w", err)
	}
	return nil
}

// Cancel discards a previewed job without committing. The transition is
// conditional on the job still being in the preview state, so a concurrent
// commit that took the slot first returns ErrJobStateConflict instead of
// racing the cancellation.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = ?, preview = NULL, finished_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`,
		StatusCancelled, id, StatusPreview,
	)
	if err != nil {
		return fmt.Errorf("cancel import job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cancel import job rows affected: %w", err)
	}
	if n == 0 {
		return ErrJobStateConflict
	}
	return nil
}

// RecoverStale marks jobs left in "processing" by a crashed process as failed,
// and returns the IDs of "pending" jobs that never started (so the caller can
// requeue them). Called once at startup before serving traffic.
func (s *Store) RecoverStale(ctx context.Context) ([]int64, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = ?, error_report = ?, finished_at = CURRENT_TIMESTAMP WHERE status = ?`,
		StatusFailed, `[{"row":0,"error":"interrupted by server restart"}]`, StatusProcessing,
	); err != nil {
		return nil, fmt.Errorf("recover stale processing jobs: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM import_jobs WHERE status = ? ORDER BY id`, StatusPending,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pending import job id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
