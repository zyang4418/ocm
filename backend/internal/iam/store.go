package iam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ocm-backend/internal/authz"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrRoleNotFound = errors.New("role not found")
	ErrCodeTaken    = errors.New("role code already taken")
	ErrNameTaken    = errors.New("name already taken")
)

// System role codes, seeded by Migrate. The admin role holds the "*"
// wildcard; all three are immutable through the API (is_system).
const (
	CodeAdmin   = "admin"
	CodeTeacher = "teacher"
	CodeStudent = "student"
)

// Store manages the RBAC tables: roles, role permissions, user/group role
// grants, direct permission grants and user groups.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the RBAC tables, seeds the system roles and migrates the
// legacy users.role column into user_roles grants. Idempotent and safe to
// run on every startup. Must run after auth.Store.Migrate (users table) and
// before any route serves traffic.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS roles (
		    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
		    code        VARCHAR(64)  NOT NULL UNIQUE,
		    name        VARCHAR(64)  NOT NULL,
		    description VARCHAR(255) NOT NULL DEFAULT '',
		    is_system   TINYINT(1)   NOT NULL DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS role_permissions (
		    role_id    BIGINT      NOT NULL,
		    permission VARCHAR(64) NOT NULL,
		    PRIMARY KEY (role_id, permission)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_roles (
		    id         BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
		    user_id    BIGINT      NOT NULL,
		    role_id    BIGINT      NOT NULL,
		    expires_at DATETIME    NULL DEFAULT NULL,
		    granted_by BIGINT      NULL DEFAULT NULL,
		    granted_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    UNIQUE KEY uq_user_role (user_id, role_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_permissions (
		    id         BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
		    user_id    BIGINT      NOT NULL,
		    permission VARCHAR(64) NOT NULL,
		    expires_at DATETIME    NULL DEFAULT NULL,
		    granted_by BIGINT      NULL DEFAULT NULL,
		    granted_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    UNIQUE KEY uq_user_perm (user_id, permission)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_groups (
		    id          BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
		    name        VARCHAR(64)  NOT NULL UNIQUE,
		    description VARCHAR(255) NOT NULL DEFAULT '',
		    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_group_members (
		    group_id BIGINT NOT NULL,
		    user_id  BIGINT NOT NULL,
		    PRIMARY KEY (group_id, user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS group_roles (
		    group_id BIGINT NOT NULL,
		    role_id  BIGINT NOT NULL,
		    PRIMARY KEY (group_id, role_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("iam migrate: %w", err)
		}
	}
	if err := s.seedSystemRoles(ctx); err != nil {
		return err
	}
	return s.migrateLegacyRoleColumn(ctx)
}

// seedSystemRoles inserts the built-in roles and (re)applies their canonical
// permission sets on every boot. System roles are immutable through the API,
// so re-seeding their permissions keeps them canonical even if the database
// was hand-edited.
func (s *Store) seedSystemRoles(ctx context.Context) error {
	seed := []Role{
		{Code: CodeAdmin, Name: "管理员", Description: "系统内置管理员角色，拥有全部权限", IsSystem: true,
			Perms: []string{authz.Wildcard}},
		{Code: CodeTeacher, Name: "教师", Description: "教师角色：查看课程与教室、预约教室、提交报修、使用 AI 助手、课堂签到", IsSystem: true,
			Perms: []string{authz.ClassroomRead, authz.CourseRead, authz.ClassroomBook,
				authz.RepairCreate, authz.AdminClassRead, authz.TeachingClassRead, authz.AiChat,
				authz.AttendanceRead, authz.AttendanceManage}},
		{Code: CodeStudent, Name: "学生", Description: "学生角色：查看课程与教室、预约教室、提交报修、扫码签到", IsSystem: true,
			Perms: []string{authz.ClassroomRead, authz.CourseRead, authz.ClassroomBook,
				authz.RepairCreate, authz.AdminClassRead, authz.TeachingClassRead,
				authz.AttendanceCheckin}},
	}
	for _, role := range seed {
		res, err := s.db.ExecContext(ctx,
			`INSERT IGNORE INTO roles (code, name, description, is_system) VALUES (?, ?, ?, ?)`,
			role.Code, role.Name, role.Description, role.IsSystem)
		if err != nil {
			return fmt.Errorf("seed role %s: %w", role.Code, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("seed role %s last insert id: %w", role.Code, err)
		}
		// INSERT IGNORE reports 0 rows + LastInsertId 0 when the row already
		// exists; fetch the real id in that case.
		if id == 0 {
			if err := s.db.QueryRowContext(ctx,
				`SELECT id FROM roles WHERE code = ?`, role.Code).Scan(&id); err != nil {
				return fmt.Errorf("seed role %s lookup: %w", role.Code, err)
			}
		}
		for _, perm := range role.Perms {
			if _, err := s.db.ExecContext(ctx,
				`INSERT IGNORE INTO role_permissions (role_id, permission) VALUES (?, ?)`,
				id, perm); err != nil {
				return fmt.Errorf("seed role %s permission %s: %w", role.Code, perm, err)
			}
		}
	}
	return nil
}

// migrateLegacyRoleColumn moves the legacy users.role values into user_roles
// grants (role='admin' → admin role, role='user' → teacher role) and drops
// the column. Fresh databases never have the column; the information_schema
// probe makes the whole step a no-op for them.
func (s *Store) migrateLegacyRoleColumn(ctx context.Context) error {
	var hasRoleCol bool
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'role'`,
	).Scan(&hasRoleCol); err != nil {
		return fmt.Errorf("iam migrate probe users.role: %w", err)
	}
	if !hasRoleCol {
		return nil
	}

	// Best-effort copy; NOT EXISTS keeps re-runs from duplicating grants, and
	// existing grants are never overwritten.
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO user_roles (user_id, role_id, granted_by, granted_at)
SELECT u.id, r.id, NULL, NOW() FROM users u
JOIN roles r ON (u.role = 'admin' AND r.code = 'admin')
            OR (u.role = 'user'  AND r.code = 'teacher')
WHERE NOT EXISTS (SELECT 1 FROM user_roles x WHERE x.user_id = u.id AND x.role_id = r.id)`); err != nil {
		return fmt.Errorf("iam migrate copy users.role: %w", err)
	}

	// MySQL has no "DROP COLUMN IF EXISTS", so ignore the unknown-column
	// error (1091) for idempotency.
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE users DROP COLUMN role`); err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1091 {
			return fmt.Errorf("iam migrate drop users.role: %w", err)
		}
	}
	return nil
}

// EffectivePermissions computes the merged permission set for a user:
// direct role grants ∪ group role grants ∪ direct permission grants, with
// expired rows filtered in SQL (and again in Effective).
func (s *Store) EffectivePermissions(ctx context.Context, userID int64) (EffectiveResult, error) {
	roles, err := s.loadAllRoles(ctx)
	if err != nil {
		return EffectiveResult{}, err
	}
	byID := make(map[int64]Role, len(roles))
	for _, role := range roles {
		byID[role.ID] = role
	}

	var directRoles, groupRoles []RoleGrant
	var directPerms []PermGrant
	var groupIDs []int64

	rows, err := s.db.QueryContext(ctx, `
SELECT role_id, expires_at FROM user_roles
WHERE user_id = ? AND (expires_at IS NULL OR expires_at > NOW())`, userID)
	if err != nil {
		return EffectiveResult{}, fmt.Errorf("effective user roles: %w", err)
	}
	for rows.Next() {
		var roleID int64
		var expiresAt *time.Time
		if err := rows.Scan(&roleID, &expiresAt); err != nil {
			_ = rows.Close()
			return EffectiveResult{}, fmt.Errorf("effective scan user role: %w", err)
		}
		if role, ok := byID[roleID]; ok {
			directRoles = append(directRoles, RoleGrant{Role: role, ExpiresAt: expiresAt})
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return EffectiveResult{}, err
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT group_id FROM user_group_members WHERE user_id = ?`, userID)
	if err != nil {
		return EffectiveResult{}, fmt.Errorf("effective group membership: %w", err)
	}
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			_ = rows.Close()
			return EffectiveResult{}, fmt.Errorf("effective scan group: %w", err)
		}
		groupIDs = append(groupIDs, groupID)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return EffectiveResult{}, err
	}

	if len(groupIDs) > 0 {
		rows, err = s.db.QueryContext(ctx, `
SELECT gr.role_id FROM user_group_members ugm
JOIN group_roles gr ON gr.group_id = ugm.group_id
WHERE ugm.user_id = ?`, userID)
		if err != nil {
			return EffectiveResult{}, fmt.Errorf("effective group roles: %w", err)
		}
		for rows.Next() {
			var roleID int64
			if err := rows.Scan(&roleID); err != nil {
				_ = rows.Close()
				return EffectiveResult{}, fmt.Errorf("effective scan group role: %w", err)
			}
			if role, ok := byID[roleID]; ok {
				groupRoles = append(groupRoles, RoleGrant{Role: role})
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return EffectiveResult{}, err
		}
	}

	rows, err = s.db.QueryContext(ctx, `
SELECT permission, expires_at FROM user_permissions
WHERE user_id = ? AND (expires_at IS NULL OR expires_at > NOW())`, userID)
	if err != nil {
		return EffectiveResult{}, fmt.Errorf("effective direct permissions: %w", err)
	}
	for rows.Next() {
		var perm string
		var expiresAt *time.Time
		if err := rows.Scan(&perm, &expiresAt); err != nil {
			_ = rows.Close()
			return EffectiveResult{}, fmt.Errorf("effective scan direct permission: %w", err)
		}
		directPerms = append(directPerms, PermGrant{Permission: perm, ExpiresAt: expiresAt})
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return EffectiveResult{}, err
	}

	return Effective(time.Now(), directRoles, groupRoles, directPerms, groupIDs), nil
}

// UserGrants returns all grants of a user for the console, including
// already-expired rows so the UI can mark them.
func (s *Store) UserGrants(ctx context.Context, userID int64) (UserGrantView, error) {
	view := UserGrantView{Roles: []RoleGrantView{}, Permissions: []PermGrantView{}, Groups: []GroupBrief{}}

	rows, err := s.db.QueryContext(ctx, `
SELECT ur.role_id, r.code, r.name, ur.expires_at
FROM user_roles ur JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ? ORDER BY r.code`, userID)
	if err != nil {
		return UserGrantView{}, fmt.Errorf("user grants roles: %w", err)
	}
	for rows.Next() {
		var v RoleGrantView
		if err := rows.Scan(&v.RoleID, &v.Code, &v.Name, &v.ExpiresAt); err != nil {
			_ = rows.Close()
			return UserGrantView{}, fmt.Errorf("user grants scan role: %w", err)
		}
		view.Roles = append(view.Roles, v)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return UserGrantView{}, err
	}

	rows, err = s.db.QueryContext(ctx, `
SELECT permission, expires_at FROM user_permissions
WHERE user_id = ? ORDER BY permission`, userID)
	if err != nil {
		return UserGrantView{}, fmt.Errorf("user grants permissions: %w", err)
	}
	for rows.Next() {
		var v PermGrantView
		if err := rows.Scan(&v.Permission, &v.ExpiresAt); err != nil {
			_ = rows.Close()
			return UserGrantView{}, fmt.Errorf("user grants scan permission: %w", err)
		}
		view.Permissions = append(view.Permissions, v)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return UserGrantView{}, err
	}

	rows, err = s.db.QueryContext(ctx, `
SELECT g.id, g.name FROM user_group_members ugm
JOIN user_groups g ON g.id = ugm.group_id
WHERE ugm.user_id = ? ORDER BY g.id`, userID)
	if err != nil {
		return UserGrantView{}, fmt.Errorf("user grants groups: %w", err)
	}
	for rows.Next() {
		var v GroupBrief
		if err := rows.Scan(&v.ID, &v.Name); err != nil {
			_ = rows.Close()
			return UserGrantView{}, fmt.Errorf("user grants scan group: %w", err)
		}
		view.Groups = append(view.Groups, v)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return UserGrantView{}, err
	}
	return view, nil
}

// GroupBriefs returns the groups a user belongs to, for identity views such
// as /api/auth/me.
func (s *Store) GroupBriefs(ctx context.Context, userID int64) ([]GroupBrief, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT g.id, g.name FROM user_group_members ugm
JOIN user_groups g ON g.id = ugm.group_id
WHERE ugm.user_id = ? ORDER BY g.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("group briefs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groups := []GroupBrief{}
	for rows.Next() {
		var g GroupBrief
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, fmt.Errorf("group briefs scan: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// BatchSummaries loads the unexpired roles and group memberships for many
// users in two queries, keyed by user id. Users without any grants get an
// empty (non-nil) summary.
func (s *Store) BatchSummaries(ctx context.Context, userIDs []int64) (map[int64]UserSummary, error) {
	summaries := make(map[int64]UserSummary, len(userIDs))
	if len(userIDs) == 0 {
		return summaries, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT ur.user_id, r.id, r.code, r.name
FROM user_roles ur JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id IN (`+placeholders+`) AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
ORDER BY ur.user_id, r.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("batch summaries roles: %w", err)
	}
	for rows.Next() {
		var userID int64
		var brief RoleBrief
		if err := rows.Scan(&userID, &brief.ID, &brief.Code, &brief.Name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("batch summaries scan role: %w", err)
		}
		s := summaries[userID]
		s.Roles = append(s.Roles, brief)
		summaries[userID] = s
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx, `
SELECT ugm.user_id, g.id, g.name
FROM user_group_members ugm JOIN user_groups g ON g.id = ugm.group_id
WHERE ugm.user_id IN (`+placeholders+`) ORDER BY ugm.user_id, g.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("batch summaries groups: %w", err)
	}
	for rows.Next() {
		var userID int64
		var brief GroupBrief
		if err := rows.Scan(&userID, &brief.ID, &brief.Name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("batch summaries scan group: %w", err)
		}
		s := summaries[userID]
		s.Groups = append(s.Groups, brief)
		summaries[userID] = s
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}

// ReplaceUserRoles replaces the whole set of direct role grants for a user
// (the console sends the complete target set). Unknown role codes fail the
// transaction with ErrRoleNotFound; duplicate codes are collapsed.
func (s *Store) ReplaceUserRoles(ctx context.Context, userID int64, grants []RoleGrantInput, grantedBy int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace user roles begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("replace user roles delete: %w", err)
	}
	seen := make(map[string]bool, len(grants))
	for _, g := range grants {
		if seen[g.RoleCode] {
			continue
		}
		seen[g.RoleCode] = true
		var roleID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE code = ?`, g.RoleCode).Scan(&roleID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRoleNotFound
		}
		if err != nil {
			return fmt.Errorf("replace user roles lookup %s: %w", g.RoleCode, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role_id, expires_at, granted_by) VALUES (?, ?, ?, ?)`,
			userID, roleID, g.ExpiresAt, grantedBy); err != nil {
			return fmt.Errorf("replace user roles insert %s: %w", g.RoleCode, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace user roles commit: %w", err)
	}
	return nil
}

// ReplaceUserPermissions replaces the whole set of direct permission grants
// for a user. Duplicate permissions are collapsed.
func (s *Store) ReplaceUserPermissions(ctx context.Context, userID int64, grants []PermGrantInput, grantedBy int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace user permissions begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_permissions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("replace user permissions delete: %w", err)
	}
	seen := make(map[string]bool, len(grants))
	for _, g := range grants {
		if seen[g.Permission] {
			continue
		}
		seen[g.Permission] = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_permissions (user_id, permission, expires_at, granted_by) VALUES (?, ?, ?, ?)`,
			userID, g.Permission, g.ExpiresAt, grantedBy); err != nil {
			return fmt.Errorf("replace user permissions insert %s: %w", g.Permission, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace user permissions commit: %w", err)
	}
	return nil
}

// DeleteUserGrants removes all grant rows of a deleted user: direct role
// grants, direct permission grants and group memberships.
func (s *Store) DeleteUserGrants(ctx context.Context, userID int64) error {
	for _, stmt := range []string{
		`DELETE FROM user_roles WHERE user_id = ?`,
		`DELETE FROM user_permissions WHERE user_id = ?`,
		`DELETE FROM user_group_members WHERE user_id = ?`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt, userID); err != nil {
			return fmt.Errorf("delete user grants: %w", err)
		}
	}
	return nil
}

// loadAllRoles loads every role with its permission set (the table is small,
// so one join instead of per-role queries).
func (s *Store) loadAllRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.id, r.code, r.name, r.description, r.is_system, rp.permission
FROM roles r LEFT JOIN role_permissions rp ON rp.role_id = r.id
ORDER BY r.id, rp.permission`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []Role
	var byID = make(map[int64]int)
	for rows.Next() {
		var id int64
		var code, name, description string
		var isSystem bool
		var perm sql.NullString
		if err := rows.Scan(&id, &code, &name, &description, &isSystem, &perm); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		idx, ok := byID[id]
		if !ok {
			roles = append(roles, Role{ID: id, Code: code, Name: name, Description: description, IsSystem: isSystem, Perms: []string{}})
			byID[id] = len(roles) - 1
			idx = len(roles) - 1
		}
		if perm.Valid {
			roles[idx].Perms = append(roles[idx].Perms, perm.String)
		}
	}
	return roles, rows.Err()
}

// UsersExist reports whether every user id exists in the users table.
// Duplicates are collapsed; empty input returns true.
func (s *Store) UsersExist(ctx context.Context, userIDs []int64) (bool, error) {
	ids := dedupeInt64(userIDs)
	if len(ids) == 0 {
		return true, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE id IN (`+placeholders+`)`, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("users exist: %w", err)
	}
	return count == len(ids), nil
}

func dedupeInt64(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// ListRoles returns every role with its permission set, ordered by id.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	return s.loadAllRoles(ctx)
}

// GetRoleByID loads one role with its permission set.
func (s *Store) GetRoleByID(ctx context.Context, id int64) (Role, error) {
	roles, err := s.loadAllRoles(ctx)
	if err != nil {
		return Role{}, err
	}
	for _, role := range roles {
		if role.ID == id {
			return role, nil
		}
	}
	return Role{}, ErrNotFound
}

// GetRoleByCode loads one role with its permission set.
func (s *Store) GetRoleByCode(ctx context.Context, code string) (Role, error) {
	roles, err := s.loadAllRoles(ctx)
	if err != nil {
		return Role{}, err
	}
	for _, role := range roles {
		if role.Code == code {
			return role, nil
		}
	}
	return Role{}, ErrRoleNotFound
}

// CreateRole inserts a role and its permission set.
func (s *Store) CreateRole(ctx context.Context, in RoleInput) (Role, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO roles (code, name, description) VALUES (?, ?, ?)`,
		in.Code, in.Name, in.Description)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return Role{}, ErrCodeTaken
		}
		return Role{}, fmt.Errorf("create role: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Role{}, fmt.Errorf("create role last insert id: %w", err)
	}
	if err := s.replaceRolePermissions(ctx, id, in.Permissions); err != nil {
		return Role{}, err
	}
	return s.GetRoleByID(ctx, id)
}

// UpdateRole updates the display fields and permission set of a role. Code
// is immutable; the caller (handler) guards is_system roles.
func (s *Store) UpdateRole(ctx context.Context, id int64, in RoleInput) (Role, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE roles SET name = ?, description = ? WHERE id = ?`,
		in.Name, in.Description, id)
	if err != nil {
		return Role{}, fmt.Errorf("update role: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Role{}, fmt.Errorf("update role rows affected: %w", err)
	}
	if n == 0 {
		if _, err := s.GetRoleByID(ctx, id); errors.Is(err, ErrNotFound) {
			return Role{}, ErrNotFound
		}
	}
	if err := s.replaceRolePermissions(ctx, id, in.Permissions); err != nil {
		return Role{}, err
	}
	return s.GetRoleByID(ctx, id)
}

func (s *Store) replaceRolePermissions(ctx context.Context, roleID int64, permissions []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace role permissions begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
		return fmt.Errorf("replace role permissions delete: %w", err)
	}
	for _, perm := range permissions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (role_id, permission) VALUES (?, ?)`,
			roleID, perm); err != nil {
			return fmt.Errorf("replace role permissions insert %s: %w", perm, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace role permissions commit: %w", err)
	}
	return nil
}

// DeleteRole removes a role and its permission rows. The caller (handler)
// refuses system roles and roles still in use.
func (s *Store) DeleteRole(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete role begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, id); err != nil {
		return fmt.Errorf("delete role permissions: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete role rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete role commit: %w", err)
	}
	return nil
}

// RoleUsageCounts counts users and groups the role is currently assigned to
// (including expired user grants, which still exist as rows).
func (s *Store) RoleUsageCounts(ctx context.Context, roleID int64) (users, groups int64, err error) {
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_roles WHERE role_id = ?`, roleID).Scan(&users); err != nil {
		return 0, 0, fmt.Errorf("role usage users: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_roles WHERE role_id = ?`, roleID).Scan(&groups); err != nil {
		return 0, 0, fmt.Errorf("role usage groups: %w", err)
	}
	return users, groups, nil
}

// ListGroups returns all groups with member counts.
func (s *Store) ListGroups(ctx context.Context) ([]GroupView, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT g.id, g.name, g.description, g.created_at, COUNT(m.user_id)
FROM user_groups g LEFT JOIN user_group_members m ON m.group_id = g.id
GROUP BY g.id, g.name, g.description, g.created_at
ORDER BY g.id`)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groups := []GroupView{}
	for rows.Next() {
		var g GroupView
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.MemberCount); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetGroupByID loads one group with members and roles (edit form prefill).
func (s *Store) GetGroupByID(ctx context.Context, id int64) (GroupDetail, error) {
	var d GroupDetail
	var count int64
	err := s.db.QueryRowContext(ctx, `
SELECT g.id, g.name, g.description, g.created_at, COUNT(m.user_id)
FROM user_groups g LEFT JOIN user_group_members m ON m.group_id = g.id
WHERE g.id = ? GROUP BY g.id, g.name, g.description, g.created_at`, id).
		Scan(&d.ID, &d.Name, &d.Description, &d.CreatedAt, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupDetail{}, ErrNotFound
	}
	if err != nil {
		return GroupDetail{}, fmt.Errorf("get group: %w", err)
	}
	d.MemberCount = count
	d.Members = []GroupMemberView{}
	d.Roles = []RoleBrief{}

	rows, err := s.db.QueryContext(ctx, `
SELECT u.id, u.username, u.display_name
FROM user_group_members m JOIN users u ON u.id = m.user_id
WHERE m.group_id = ? ORDER BY u.id`, id)
	if err != nil {
		return GroupDetail{}, fmt.Errorf("get group members: %w", err)
	}
	for rows.Next() {
		var m GroupMemberView
		if err := rows.Scan(&m.ID, &m.Username, &m.DisplayName); err != nil {
			_ = rows.Close()
			return GroupDetail{}, fmt.Errorf("get group scan member: %w", err)
		}
		d.Members = append(d.Members, m)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return GroupDetail{}, err
	}

	rows, err = s.db.QueryContext(ctx, `
SELECT r.id, r.code, r.name FROM group_roles gr
JOIN roles r ON r.id = gr.role_id
WHERE gr.group_id = ? ORDER BY r.id`, id)
	if err != nil {
		return GroupDetail{}, fmt.Errorf("get group roles: %w", err)
	}
	for rows.Next() {
		var brief RoleBrief
		if err := rows.Scan(&brief.ID, &brief.Code, &brief.Name); err != nil {
			_ = rows.Close()
			return GroupDetail{}, fmt.Errorf("get group scan role: %w", err)
		}
		d.Roles = append(d.Roles, brief)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return GroupDetail{}, err
	}
	return d, nil
}

// CreateGroup inserts a group with its members and roles.
func (s *Store) CreateGroup(ctx context.Context, in GroupInput) (GroupView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GroupView{}, fmt.Errorf("create group begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO user_groups (name, description) VALUES (?, ?)`,
		in.Name, in.Description)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return GroupView{}, ErrNameTaken
		}
		return GroupView{}, fmt.Errorf("create group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GroupView{}, fmt.Errorf("create group last insert id: %w", err)
	}
	if err := replaceGroupMembers(tx, ctx, id, in.Members); err != nil {
		return GroupView{}, err
	}
	if err := replaceGroupRoles(tx, ctx, id, in.Roles); err != nil {
		return GroupView{}, err
	}
	if err := tx.Commit(); err != nil {
		return GroupView{}, fmt.Errorf("create group commit: %w", err)
	}

	var g GroupView
	if err := s.db.QueryRowContext(ctx, `
SELECT id, name, description, created_at FROM user_groups WHERE id = ?`, id).
		Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
		return GroupView{}, fmt.Errorf("create group reload: %w", err)
	}
	g.MemberCount = int64(len(in.Members))
	return g, nil
}

// UpdateGroup replaces the display fields, members and roles of a group.
func (s *Store) UpdateGroup(ctx context.Context, id int64, in GroupInput) (GroupView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GroupView{}, fmt.Errorf("update group begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE user_groups SET name = ?, description = ? WHERE id = ?`,
		in.Name, in.Description, id)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return GroupView{}, ErrNameTaken
		}
		return GroupView{}, fmt.Errorf("update group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return GroupView{}, fmt.Errorf("update group rows affected: %w", err)
	}
	if n == 0 {
		if _, err := s.GetGroupByID(ctx, id); errors.Is(err, ErrNotFound) {
			return GroupView{}, ErrNotFound
		}
	}
	if err := replaceGroupMembers(tx, ctx, id, in.Members); err != nil {
		return GroupView{}, err
	}
	if err := replaceGroupRoles(tx, ctx, id, in.Roles); err != nil {
		return GroupView{}, err
	}
	if err := tx.Commit(); err != nil {
		return GroupView{}, fmt.Errorf("update group commit: %w", err)
	}

	d, err := s.GetGroupByID(ctx, id)
	if err != nil {
		return GroupView{}, err
	}
	return d.GroupView, nil
}

func replaceGroupMembers(tx *sql.Tx, ctx context.Context, groupID int64, memberIDs []int64) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_group_members WHERE group_id = ?`, groupID); err != nil {
		return fmt.Errorf("replace group members delete: %w", err)
	}
	for _, userID := range dedupeInt64(memberIDs) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_group_members (group_id, user_id) VALUES (?, ?)`,
			groupID, userID); err != nil {
			return fmt.Errorf("replace group members insert %d: %w", userID, err)
		}
	}
	return nil
}

func replaceGroupRoles(tx *sql.Tx, ctx context.Context, groupID int64, roleIDs []int64) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM group_roles WHERE group_id = ?`, groupID); err != nil {
		return fmt.Errorf("replace group roles delete: %w", err)
	}
	for _, roleID := range dedupeInt64(roleIDs) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO group_roles (group_id, role_id) VALUES (?, ?)`,
			groupID, roleID); err != nil {
			return fmt.Errorf("replace group roles insert %d: %w", roleID, err)
		}
	}
	return nil
}

// DeleteGroup removes a group with its members and role grants.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete group begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_group_members WHERE group_id = ?`, id); err != nil {
		return fmt.Errorf("delete group members: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_roles WHERE group_id = ?`, id); err != nil {
		return fmt.Errorf("delete group roles: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM user_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete group rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete group commit: %w", err)
	}
	return nil
}
