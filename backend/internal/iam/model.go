// Package iam implements the identity-and-access management layer: roles,
// role permissions, user/group role grants and direct permission grants,
// stored in MySQL. Effective permissions (direct role grants ∪ group role
// grants ∪ direct permission grants, minus expired rows) are computed per
// request by user.LoadSubject so grant changes take effect immediately.
package iam

import "time"

// Role is a row of the roles table plus the permissions granted to it.
type Role struct {
	ID          int64    `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsSystem    bool     `json:"isSystem"`
	Perms       []string `json:"permissions"`
}

// RoleGrant is one user↔role or group↔role grant. ExpiresAt nil means the
// grant never expires.
type RoleGrant struct {
	Role      Role
	ExpiresAt *time.Time
}

// PermGrant is one direct user permission grant. ExpiresAt nil means the
// grant never expires.
type PermGrant struct {
	Permission string
	ExpiresAt  *time.Time
}

// EffectiveResult is the output of Effective(): the merged permission set,
// the de-duplicated roles that contributed to it (with names, for direct use
// by /api/auth/me), and the de-duplicated group IDs.
type EffectiveResult struct {
	Permissions map[string]bool
	Roles       []Role
	GroupIDs    []int64
}

// RoleCodes returns the role codes of the result, for the authz.Subject.
func (r EffectiveResult) RoleCodes() []string {
	codes := make([]string, 0, len(r.Roles))
	for _, role := range r.Roles {
		codes = append(codes, role.Code)
	}
	return codes
}

// ---- JSON view types ----

// RoleBrief is a compact role reference embedded in user/group responses.
type RoleBrief struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// GroupBrief is a compact group reference embedded in user responses.
type GroupBrief struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// RoleGrantView is one role grant as returned to the console, including
// already-expired rows (the UI marks them).
type RoleGrantView struct {
	RoleID    int64      `json:"roleId"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// PermGrantView is one direct permission grant as returned to the console,
// including already-expired rows.
type PermGrantView struct {
	Permission string     `json:"permission"`
	ExpiresAt  *time.Time `json:"expiresAt"`
}

// UserGrantView is the response of GET /api/users/{id}/grants.
type UserGrantView struct {
	Roles       []RoleGrantView `json:"roles"`
	Permissions []PermGrantView `json:"permissions"`
	Groups      []GroupBrief    `json:"groups"`
}

// UserSummary carries a user's roles/groups for list responses, filled by
// one batch query per page.
type UserSummary struct {
	Roles  []RoleBrief
	Groups []GroupBrief
}

// GroupView is one row of the groups list (member count filled by the store).
type GroupView struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	MemberCount int64     `json:"memberCount"`
}

// GroupMemberView is a member reference inside a group detail.
type GroupMemberView struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

// GroupDetail is the response of GET /api/groups/{id} (edit form prefill).
type GroupDetail struct {
	GroupView
	Members []GroupMemberView `json:"members"`
	Roles   []RoleBrief       `json:"roles"`
}

// ---- Write inputs ----

// RoleInput creates or updates a role. Code is immutable after creation
// (PUT ignores it); Permissions must all exist in the authz catalog.
type RoleInput struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// GroupInput creates or updates a group, replacing members and roles.
type GroupInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Members     []int64 `json:"members"`
	Roles       []int64 `json:"roles"`
}

// RoleGrantInput is one element of a replace-set role assignment.
type RoleGrantInput struct {
	RoleCode  string     `json:"roleCode"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// PermGrantInput is one element of a replace-set permission assignment.
type PermGrantInput struct {
	Permission string     `json:"permission"`
	ExpiresAt  *time.Time `json:"expiresAt"`
}
