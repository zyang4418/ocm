package systemlog

import (
	"context"
	"net/http"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
)

// annotation carries the optional business summary of the audited request.
// Handlers write into the holder via WithSummary; the middleware reads it
// after the handler returns. A pointer holder is required because Go contexts
// flow downward only — an upstream middleware cannot see a downstream
// context value.
type annotation struct{ summary string }
type annotationKey struct{}

// WithSummary annotates the current request's audit row with a business
// description (e.g. "删除教室 A101"). A no-op when the request is not being
// audited (no holder in ctx).
func WithSummary(ctx context.Context, summary string) {
	if a, ok := ctx.Value(annotationKey{}).(*annotation); ok {
		a.summary = summary
	}
}

// Audit records every mutating request (POST/PUT/PATCH/DELETE) that reaches
// it into system_logs: actor, method, path, status, client IP, plus the
// optional summary annotated by the handler. It must run inside the auth
// pipeline — after LoadSubject (so the Subject is available) and before
// RequirePermission (so 403 rejections are recorded too). Requests rejected
// upstream by auth.Middleware/LoadSubject (401) never reach it and are not
// recorded: they have no attributable actor, and an outer middleware cannot
// see the inner request context anyway.
func Audit(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPut &&
				r.Method != http.MethodPatch && r.Method != http.MethodDelete {
				next.ServeHTTP(w, r)
				return
			}
			rec := httpx.NewStatusRecorder(w)
			ann := &annotation{}
			ctx := context.WithValue(r.Context(), annotationKey{}, ann)
			subject, _ := authz.SubjectFrom(r.Context())
			// defer so a panicking handler still produces a row (status 500,
			// matching httpx.Recover's response).
			defer func() {
				code := rec.Code()
				if code == 0 {
					code = http.StatusInternalServerError
				}
				store.Insert(ctx, Entry{
					ActorID:    subject.ID,
					ActorName:  subject.DisplayName,
					Method:     r.Method,
					Path:       r.URL.Path,
					StatusCode: code,
					Summary:    ann.summary,
					ClientIP:   httpx.ClientIP(r),
				})
			}()
			next.ServeHTTP(rec, r.WithContext(ctx))
		})
	}
}
