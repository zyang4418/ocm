package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"ocm-backend/internal/dbutil"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrJobNotFound      = errors.New("import job not found")
	ErrJobStateConflict = errors.New("import job not in expected state")
)

const jobColumns = `id, type, status, filename, payload, total_rows, succeeded_rows, failed_rows, error_report, user_id, created_at, started_at, finished_at`

// jobMetaColumns excludes payload (the base64 xlsx, up to 5MB), the preview blob
// (the full dry-run rows, potentially tens of thousands), and error_report (a
// per-row error JSON that can reach several MB for a sessions job whose rows
// all fail) so GET /api/imports/{id} — polled by the wizard while a job
// processes — stays small. Preview rows are served page-by-page by GetJobRows;
// the error report is served on demand by GetJobErrors.
const jobMetaColumns = `id, type, status, filename, total_rows, succeeded_rows, failed_rows, user_id, created_at, started_at, finished_at`

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

// JobSpec is one import job to create as part of an atomic batch.
type JobSpec struct {
	Type     string
	Filename string
	Payload  string
}

// CreateJobs inserts every spec in a single transaction: either all job rows
// are created or none are. It returns the freshly-created jobs in order. The
// JWC splitter produces 6 jobs in one request; without an atomic batch a
// mid-loop failure would leave already-created jobs running with no way for
// the operator to discover them from the error response.
func (s *Store) CreateJobs(ctx context.Context, userID int64, specs []JobSpec) ([]Job, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO import_jobs (type, status, filename, payload, error_report, user_id) VALUES (?, 'pending', ?, ?, '', ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare create job: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	ids := make([]int64, 0, len(specs))
	for _, sp := range specs {
		res, err := stmt.ExecContext(ctx, sp.Type, sp.Filename, sp.Payload, userID)
		if err != nil {
			return nil, fmt.Errorf("create import job: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("create import job last insert id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create jobs: %w", err)
	}
	out := make([]Job, 0, len(ids))
	for _, id := range ids {
		j, err := s.GetJob(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get created job %d: %w", id, err)
		}
		out = append(out, j)
	}
	return out, nil
}

func (s *Store) GetJob(ctx context.Context, id int64) (Job, error) {
	var j Job
	var started, finished sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM import_jobs WHERE id = ?`, id,
	).Scan(
		&j.ID, &j.Type, &j.Status, &j.Filename, &j.Payload, &j.TotalRows, &j.SucceededRows, &j.FailedRows, &j.ErrorReport, &j.UserID, &j.CreatedAt, &started, &finished,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("get import job: %w", err)
	}
	// Preview rows are intentionally not loaded here: GetJob backs the
	// commit/reanalyze paths (which need Payload, not rows), and the rows are
	// served page-by-page via GetJobRows to avoid decoding a multi-thousand-row
	// preview on every call.
	if started.Valid {
		j.StartedAt = &started.Time
	}
	if finished.Valid {
		j.FinishedAt = &finished.Time
	}
	return j, nil
}

// GetJobMeta returns a job's metadata without the payload or preview blob. It
// backs GET /api/imports/{id}, which the wizard polls while a job processes;
// excluding payload (≤5MB xlsx) and preview (potentially tens of thousands of
// rows) keeps each poll response small.
func (s *Store) GetJobMeta(ctx context.Context, id int64) (Job, error) {
	var j Job
	var started, finished sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT `+jobMetaColumns+` FROM import_jobs WHERE id = ?`, id,
	).Scan(
		&j.ID, &j.Type, &j.Status, &j.Filename, &j.TotalRows, &j.SucceededRows, &j.FailedRows, &j.UserID, &j.CreatedAt, &started, &finished,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("get import job meta: %w", err)
	}
	if started.Valid {
		j.StartedAt = &started.Time
	}
	if finished.Valid {
		j.FinishedAt = &finished.Time
	}
	return j, nil
}

// GetJobRows returns one page of a job's preview rows plus the total preview
// row count. The preview is stored as a JSON array in import_jobs.preview; it is
// decoded into []json.RawMessage (one pass, no per-row map allocation for the
// whole set) and only the requested page is unmarshaled into maps. Returns
// ([], 0, nil) when the job has no preview (committed/cancelled/failed-early).
func (s *Store) GetJobRows(ctx context.Context, id int64, limit, offset int) ([]map[string]any, int, error) {
	var preview sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT preview FROM import_jobs WHERE id = ?`, id).Scan(&preview)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrJobNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("get import preview: %w", err)
	}
	if !preview.Valid || preview.String == "" || preview.String == "null" {
		return []map[string]any{}, 0, nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(preview.String), &raw); err != nil {
		// Malformed preview: surface as empty so the pager still renders.
		return []map[string]any{}, 0, nil
	}
	total := len(raw)
	if limit < 1 {
		limit = 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := make([]map[string]any, 0, end-offset)
	for _, r := range raw[offset:end] {
		var m map[string]any
		if err := json.Unmarshal(r, &m); err != nil {
			m = map[string]any{}
		}
		page = append(page, m)
	}
	return page, total, nil
}

// GetJobErrors returns a job's per-row error report. error_report is a JSON
// array of RowError and can reach several MB for a large sessions job whose
// rows all fail, so it is excluded from the polled list and meta queries
// (PageJobs, GetJobMeta) and served here on demand by GET
// /api/imports/{id}/errors. Returns an empty slice (not nil) when the job has
// no errors or a malformed report.
func (s *Store) GetJobErrors(ctx context.Context, id int64) ([]RowError, error) {
	var report sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT error_report FROM import_jobs WHERE id = ?`, id).Scan(&report)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get import job errors: %w", err)
	}
	if !report.Valid || report.String == "" || report.String == "null" {
		return []RowError{}, nil
	}
	var errs []RowError
	if err := json.Unmarshal([]byte(report.String), &errs); err != nil {
		return []RowError{}, nil
	}
	if errs == nil {
		errs = []RowError{}
	}
	return errs, nil
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

// PageJobs returns one page of import jobs matching q (fuzzy contains on
// filename and type), newest first, plus the total matching count across all
// pages. The payload column is excluded to keep responses small. A zero
// Pagination means no limit (full range).
func (s *Store) PageJobs(ctx context.Context, q string, p dbutil.Pagination) ([]Job, int64, error) {
	where := ` WHERE 1=1`
	var args []any
	if q != "" {
		where += ` AND (filename LIKE ? OR type LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat)
	}
	query, queryArgs := p.AppendLimit(
		`SELECT id, type, status, filename, total_rows, succeeded_rows, failed_rows, user_id, created_at, started_at, finished_at FROM import_jobs`+
			where+` ORDER BY created_at DESC, id DESC`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	list := []Job{}
	for rows.Next() {
		var j Job
		var started, finished sql.NullTime
		if err := rows.Scan(
			&j.ID, &j.Type, &j.Status, &j.Filename, &j.TotalRows, &j.SucceededRows, &j.FailedRows, &j.UserID, &j.CreatedAt, &started, &finished,
		); err != nil {
			return nil, 0, fmt.Errorf("scan import job: %w", err)
		}
		if started.Valid {
			j.StartedAt = &started.Time
		}
		if finished.Valid {
			j.FinishedAt = &finished.Time
		}
		list = append(list, j)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total, err := dbutil.CountRows(ctx, s.db, `FROM import_jobs`+where, args)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
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
