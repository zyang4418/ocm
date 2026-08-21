package user

import (
	"time"

	"ocm-backend/internal/iam"
)

// User is the management view of an account stored in the users table.
// Roles/Groups are only filled by the list endpoint (batch summary); other
// endpoints leave them empty.
type User struct {
	ID          int64            `json:"id"`
	Username    string           `json:"username"`
	DisplayName string           `json:"displayName"`
	Type        string           `json:"type"`
	CreatedAt   time.Time        `json:"createdAt"`
	Roles       []iam.RoleBrief  `json:"roles,omitempty"`
	Groups      []iam.GroupBrief `json:"groups,omitempty"`
}

type CreateUserInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
}

type UpdateUserInput struct {
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
}

type ChangePasswordInput struct {
	Password string `json:"password"`
}

// UserRolesInput documents the PUT /api/users/{id}/roles body: a whole-set
// replace of direct role grants.
type UserRolesInput struct {
	Roles []iam.RoleGrantInput `json:"roles"`
}

// UserPermissionsInput documents the PUT /api/users/{id}/permissions body: a
// whole-set replace of direct permission grants.
type UserPermissionsInput struct {
	Permissions []iam.PermGrantInput `json:"permissions"`
}
