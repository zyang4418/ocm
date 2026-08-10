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
	UserManage      = "user:manage"
	ClassroomRead   = "classroom:read"
	ClassroomManage = "classroom:manage"
	ClassroomBook   = "classroom:book"
	BookingApprove  = "booking:approve"
	CourseRead       = "course:read"
	CourseManage     = "course:manage"
	AdminClassRead   = "admin_class:read"
	AdminClassManage = "admin_class:manage"
	// TeachingClassRead/Manage govern 教学班 (a named group of admin classes
	// that an offering is taught to). Kept separate from CourseManage so the
	// class-grouping domain stays independent of course management.
	TeachingClassRead   = "teaching_class:read"
	TeachingClassManage = "teaching_class:manage"
	RepairCreate     = "repair:create"
	RepairAssign     = "repair:assign"
)

// Subject is the authenticated actor resolved by the auth pipeline. It is
// authentication-agnostic: the same shape is produced whether the user
// arrived via JWT (web console) or openid binding (mini-program).
type Subject struct {
	ID   int64
	Role string
	// Phase 2 will add per-user extra permissions here.
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

// rolePermissions maps each base role to the permissions it grants. This is
// the single source of truth for "what can each role do"; Phase 3 will move
// it to the database. The admin role is handled as a wildcard in Can.
var rolePermissions = map[string]map[string]bool{
	"user": {
		ClassroomRead:      true,
		CourseRead:         true,
		ClassroomBook:      true,
		RepairCreate:       true,
		AdminClassRead:     true,
		TeachingClassRead:  true,
	},
}

// Can reports whether role is allowed to perform permission. The admin role
// passes everything; roles absent from the map pass nothing.
func Can(role, permission string) bool {
	if role == "admin" {
		return true
	}
	return rolePermissions[role][permission]
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
			if !Can(s.Role, permission) {
				httpx.RespondError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
