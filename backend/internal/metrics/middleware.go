package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/httpx"
)

// reportUnmatched is the path label for requests the mux dispatched to its
// own NotFound handler: no pattern matched, and labelling by raw URL would
// blow up cardinality.
const reportUnmatched = "unmatched"

// Middleware instruments each request with RED metrics. Placement mirrors
// AccessLog: it must stay OUTSIDE Recover so a recovered panic lands as a 500
// sample rather than vanishing, and inside AccessLog, which owns logging.
// Probes are skipped for the same flood reason as in AccessLog.
//
// The route label reads r.Pattern after the inner handler returns: ServeMux
// sets it on the request before dispatching, so by the time this outer
// handler resumes, the matched pattern is visible on the same *Request.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		InFlight.Inc()
		defer InFlight.Dec()
		rec := httpx.NewStatusRecorder(w)
		start := time.Now()
		next.ServeHTTP(rec, r)
		route := routePattern(r)
		RequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.Code())).Inc()
		RequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// routePattern extracts the low-cardinality route label. Method-routed
// patterns ("GET /api/roles/{id}") repeat the method the metric already
// carries as a label, so only the path part is kept; methodless patterns
// ("/api/") and unmatched requests pass through as-is / as reportUnmatched.
func routePattern(r *http.Request) string {
	pattern := r.Pattern
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		pattern = pattern[i+1:]
	}
	if pattern == "" {
		return reportUnmatched
	}
	return pattern
}
