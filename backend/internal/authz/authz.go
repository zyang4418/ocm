package authz

import (
	"context"
	"net/http"

	"ocm-backend/internal/httpx"
)

// Permission constants are the stable contract between handlers and the
// authorization model. Handlers check these strings, never role names, so
// the role-to-permission mapping can evolve without touching handler code.
const (
	UserManage       = "user:manage"
	UserRead         = "user:read"
	RoleRead         = "role:read"
	RoleManage       = "role:manage"
	GroupRead        = "group:read"
	GroupManage      = "group:manage"
	ClassroomRead    = "classroom:read"
	ClassroomManage  = "classroom:manage"
	ClassroomBook    = "classroom:book"
	BookingApprove   = "booking:approve"
	CourseRead       = "course:read"
	CourseManage     = "course:manage"
	AdminClassRead   = "admin_class:read"
	AdminClassManage = "admin_class:manage"
	// TeachingClassRead/Manage govern 教学班 (a named group of admin classes
	// that an offering is taught to). Kept separate from CourseManage so the
	// class-grouping domain stays independent of course management.
	TeachingClassRead   = "teaching_class:read"
	TeachingClassManage = "teaching_class:manage"
	RepairCreate        = "repair:create"
	RepairAssign        = "repair:assign"
	LogRead             = "log:read"
	LogManage           = "log:manage"
	AiChat              = "ai:chat"
	AttendanceRead      = "attendance:read"
	AttendanceManage    = "attendance:manage"
	AttendanceCheckin   = "attendance:checkin"
	// ObservationRead/Write/Manage govern 听课评课. Read sees one's own records;
	// Write creates/edits/submits/deletes one's own drafts; Manage (教务/督导)
	// sees every record and exports any submitted one.
	ObservationRead   = "observation:read"
	ObservationWrite  = "observation:write"
	ObservationManage = "observation:manage"
)

// Wildcard is the special permission that grants everything. It is only held
// by the system admin role (seeded as a role_permissions row); the catalog
// deliberately does not contain it so API validation can never grant it.
const Wildcard = "*"

// Subject is the authenticated actor resolved by the auth pipeline. It is
// authentication-agnostic: the same shape is produced whether the user
// arrived via JWT (web console) or openid binding (mini-program).
//
// Permissions is the merged effective set (direct role grants ∪ group role
// grants ∪ direct permission grants), already filtered for expired grants.
// Roles/Groups carry the de-duplicated role codes and group IDs that produced
// it, for display purposes. Username/DisplayName identify the actor for
// audit records (systemlog).
type Subject struct {
	ID          int64
	Type        string
	Username    string
	DisplayName string
	Permissions map[string]bool
	Roles       []string
	Groups      []int64
}

// Has reports whether the subject holds the given permission. A subject
// holding the "*" wildcard passes everything.
func (s Subject) Has(permission string) bool {
	return s.Permissions[Wildcard] || s.Permissions[permission]
}

type subjectKey struct{}

// WithSubject stores s in ctx for downstream authorization checks.
func WithSubject(ctx context.Context, s Subject) context.Context {
	return context.WithValue(ctx, subjectKey{}, s)
}

// SubjectFrom extracts the Subject placed in ctx by the auth pipeline.
func SubjectFrom(ctx context.Context) (Subject, bool) {
	s, ok := ctx.Value(subjectKey{}).(Subject)
	return s, ok
}

// RequirePermission returns a middleware that rejects requests whose Subject
// lacks the given permission. It must run after the auth pipeline has
// populated the Subject in the request context.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := SubjectFrom(r.Context())
			if !ok {
				httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			if !s.Has(permission) {
				httpx.RespondError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin returns a middleware that admits a request only when the
// Subject holds the "*" wildcard — the seeded system admin role. It is
// stricter than RequirePermission: a regular permission can be granted to
// other roles, the wildcard cannot (it is absent from the catalog). Used for
// system-level configuration (e.g. mail/storage settings) that must stay
// admin-only regardless of role grants. Must run after the auth pipeline has
// populated the Subject in the request context.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := SubjectFrom(r.Context())
		if !ok {
			httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !s.Has(Wildcard) {
			httpx.RespondError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAny returns a middleware that admits a request when the Subject
// holds at least one of the given permissions. Used for catalog/list
// endpoints (e.g. role and group listings) that several management roles
// need to read. It must run after the auth pipeline has populated the
// Subject in the request context.
func RequireAny(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := SubjectFrom(r.Context())
			if !ok {
				httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			for _, perm := range permissions {
				if s.Has(perm) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.RespondError(w, http.StatusForbidden, "insufficient permissions")
		})
	}
}
