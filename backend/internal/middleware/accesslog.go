// Package middleware holds cross-cutting HTTP middleware that composes
// httpx/logging primitives without creating import cycles.
package middleware

import (
	"net/http"
	"time"

	"ocm-backend/internal/httpx"
	"ocm-backend/internal/logging"
)

// AccessLog logs one structured line per request: method, path, status,
// duration, authenticated user, request id and client IP. It must be the
// OUTERMOST handler (outside httpx.Recover) so panics recovered by Recover
// still get their final 500 recorded here; with the order inverted a panic
// unwinds past this middleware before Recover writes the response and the
// line would show status 0.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Probes would flood the log on every platform restart.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, id := logging.NewRequestContext(r.Context())
		rec := httpx.NewStatusRecorder(w)
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.Code(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", id,
			"ip", httpx.ClientIP(r),
		}
		if user := logging.UserFrom(ctx); user != "" {
			attrs = append(attrs, "user", user)
		}
		if rec.Code() >= 500 {
			logging.L.Error("http: request", attrs...)
		} else {
			logging.L.Info("http: request", attrs...)
		}
	})
}
