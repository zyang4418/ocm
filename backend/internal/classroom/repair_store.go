package classroom

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ocm-backend/internal/dbutil"
)

// RepairStore manages repair tickets in the classroom_repairs table. It depends
// only on the shared *sql.DB; classroom/creator/assignee lookups join the
// classrooms and users tables directly, matching how booking and observation
// resolve their cross-entity references.
type RepairStore struct {
	db *sql.DB
}

func NewRepairStore(db *sql.DB) *RepairStore { return &RepairStore{db: db} }

// Migrate creates the classroom_repairs table. Idempotent, safe to run on
// every startup. images is stored as JSON text (a URL list) and defaults to
// "[]"; nothing uploads into it yet.
func (s *RepairStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS classroom_repairs (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    classroom_id BIGINT      NOT NULL,
    creator_id   BIGINT      NOT NULL,
    assignee_id  BIGINT      NULL DEFAULT NULL,
    description  TEXT        NOT NULL,
    images       LONGTEXT    NOT NULL,
    status       VARCHAR(16) NOT NULL DEFAULT 'open',
    remark       TEXT        NOT NULL,
    created_at   TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_repair_classroom_status (classroom_id, status),
    INDEX idx_repair_creator (creator_id),
    INDEX idx_repair_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create classroom_repairs table: %w", err)
	}
	return nil
}

// ---- resolution helpers ----

func (s *RepairStore) classroomExists(ctx context.Context, id int64) error {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM classrooms WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check classroom exists: %w", err)
	}
	return nil
}

// hasOpen reports whether the classroom already has an open or processing
// ticket. Ordinary tickets are gated on this; emergency tickets skip it.
func (s *RepairStore) hasOpen(ctx context.Context, classroomID int64) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM classroom_repairs WHERE classroom_id = ? AND status IN (?, ?) LIMIT 1`,
		classroomID, RepairStatusOpen, RepairStatusProcessing,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check open repair: %w", err)
	}
	return true, nil
}

// Create inserts an open repair ticket for the classroom, refusing when the
// classroom already has an open/processing ticket.
func (s *RepairStore) Create(ctx context.Context, in RepairInput, creatorID int64) (RepairView, error) {
	if err := s.classroomExists(ctx, in.ClassroomID); err != nil {
		return RepairView{}, err
	}
	open, err := s.hasOpen(ctx, in.ClassroomID)
	if err != nil {
		return RepairView{}, err
	}
	if open {
		return RepairView{}, ErrRepairOpenExists
	}
	return s.insert(ctx, in, creatorID)
}

// EmergencyCreate inserts an open ticket without the duplicate-open gate: an
// emergency expresses higher-priority immediate handling, so it never blocks
// on an existing open ticket.
func (s *RepairStore) EmergencyCreate(ctx context.Context, in RepairInput, creatorID int64) (RepairView, error) {
	if err := s.classroomExists(ctx, in.ClassroomID); err != nil {
		return RepairView{}, err
	}
	return s.insert(ctx, in, creatorID)
}

func (s *RepairStore) insert(ctx context.Context, in RepairInput, creatorID int64) (RepairView, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO classroom_repairs (classroom_id, creator_id, description, images, status, remark)
		 VALUES (?, ?, ?, ?, ?, '')`,
		in.ClassroomID, creatorID, in.Description, rawOrDefault(in.Images, "[]"), RepairStatusOpen,
	)
	if err != nil {
		return RepairView{}, fmt.Errorf("create repair: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return RepairView{}, fmt.Errorf("create repair last insert id: %w", err)
	}
	return s.Get(ctx, id)
}

// Update advances an open/processing ticket to processing or completed by an
// assignee (repair:assign). It records the assignee and an optional remark. A
// confirmed ticket is terminal and cannot be moved.
func (s *RepairStore) Update(ctx context.Context, id int64, in RepairUpdateInput, assigneeID int64) (RepairView, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return RepairView{}, err
	}
	if existing.Status == RepairStatusConfirmed {
		return RepairView{}, ErrRepairState
	}
	if in.Status != RepairStatusProcessing && in.Status != RepairStatusCompleted {
		return RepairView{}, ErrRepairState
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE classroom_repairs SET status = ?, remark = ?, assignee_id = ? WHERE id = ?`,
		in.Status, in.Remark, assigneeID, id,
	); err != nil {
		return RepairView{}, fmt.Errorf("update repair: %w", err)
	}
	return s.Get(ctx, id)
}

// Confirm marks a completed ticket confirmed. Only the creator may confirm;
// the transition is idempotent (an already-confirmed ticket is returned
// unchanged).
func (s *RepairStore) Confirm(ctx context.Context, id, creatorID int64) (RepairView, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return RepairView{}, err
	}
	if existing.CreatorID != creatorID {
		return RepairView{}, ErrRepairForbidden
	}
	if existing.Status == RepairStatusConfirmed {
		return existing, nil
	}
	if existing.Status != RepairStatusCompleted {
		return RepairView{}, ErrRepairState
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE classroom_repairs SET status = ? WHERE id = ? AND status = ?`,
		RepairStatusConfirmed, id, RepairStatusCompleted,
	); err != nil {
		return RepairView{}, fmt.Errorf("confirm repair: %w", err)
	}
	return s.Get(ctx, id)
}

// ---- queries ----

const repairColumns = `r.id, r.classroom_id, r.creator_id, r.assignee_id, r.description, r.images, r.status, r.remark, r.created_at, r.updated_at`

const repairJoin = `
FROM classroom_repairs r
	JOIN classrooms c ON c.id = r.classroom_id
	JOIN users cu ON cu.id = r.creator_id
	LEFT JOIN users au ON au.id = r.assignee_id`

// Get returns one repair ticket with its joined display fields.
func (s *RepairStore) Get(ctx context.Context, id int64) (RepairView, error) {
	v, err := scanRepairView(s.db.QueryRowContext(ctx,
		`SELECT `+repairColumns+`, c.name, c.building, cu.display_name, au.display_name`+repairJoin+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return RepairView{}, ErrRepairNotFound
	}
	if err != nil {
		return RepairView{}, fmt.Errorf("get repair: %w", err)
	}
	return v, nil
}

// Page returns one page of repair tickets. admin=true sees every ticket;
// otherwise the caller is scoped to their own. f filters by classroom/status,
// and q fuzzy-searches the description, classroom name and creator name.
func (s *RepairStore) Page(ctx context.Context, f RepairFilter, q string, viewerID int64, admin bool, p dbutil.Pagination) ([]RepairView, int64, error) {
	where := ` WHERE 1=1`
	var args []any
	if !admin {
		where += ` AND r.creator_id = ?`
		args = append(args, viewerID)
	}
	if f.ClassroomID > 0 {
		where += ` AND r.classroom_id = ?`
		args = append(args, f.ClassroomID)
	}
	if f.Status != "" {
		where += ` AND r.status = ?`
		args = append(args, f.Status)
	}
	if len(f.Statuses) > 0 {
		marks := strings.Repeat("?,", len(f.Statuses))
		where += ` AND r.status IN (` + marks[:len(marks)-1] + `)`
		for _, st := range f.Statuses {
			args = append(args, st)
		}
	}
	if q != "" {
		where += ` AND (r.description LIKE ? OR c.name LIKE ? OR cu.display_name LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat, pat)
	}
	query, queryArgs := p.AppendLimit(
		`SELECT `+repairColumns+`, c.name, c.building, cu.display_name, au.display_name`+repairJoin+where+` ORDER BY r.created_at DESC, r.id DESC`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page repairs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	list := []RepairView{}
	for rows.Next() {
		v, err := scanRepairView(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan repair: %w", err)
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total, err := dbutil.CountRows(ctx, s.db, repairJoin+where, args)
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func scanRepairView(sc interface{ Scan(dest ...any) error }) (RepairView, error) {
	var v RepairView
	var assigneeID sql.NullInt64
	var assigneeName sql.NullString
	var images string
	err := sc.Scan(
		&v.ID, &v.ClassroomID, &v.CreatorID, &assigneeID, &v.Description, &images, &v.Status, &v.Remark, &v.CreatedAt, &v.UpdatedAt,
		&v.ClassroomName, &v.Building, &v.CreatorName, &assigneeName,
	)
	if err != nil {
		return v, err
	}
	if assigneeID.Valid {
		id := assigneeID.Int64
		v.AssigneeID = &id
	}
	v.AssigneeName = assigneeName.String
	v.Images = jsonRaw(images, "[]")
	return v, nil
}

// ---- small JSON helpers (mirror observation's) ----

func jsonRaw(raw, dflt string) json.RawMessage {
	if raw == "" || raw == "null" {
		return json.RawMessage(dflt)
	}
	return json.RawMessage(raw)
}

func rawOrDefault(v json.RawMessage, dflt string) string {
	if len(v) == 0 || string(v) == "null" {
		return dflt
	}
	return string(v)
}
