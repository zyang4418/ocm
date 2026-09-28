// Package metrics provides the process-level observability signals: a RED
// (rate/error/duration) HTTP middleware backed by prometheus/client_golang,
// database pool gauges, and a standalone /metrics endpoint. The endpoint is
// infrastructure-free — it emits plain Prometheus text over an internal-only
// listener (METRICS_ADDR) that every deployment scenario consumes its own
// way: the compose observability overlay scrapes it, serverless platforms
// proxy or poll it. It carries no auth by design; it must never be published
// to a public interface.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// RequestsTotal counts completed requests. The path label carries the
	// ServeMux route pattern ("GET /api/roles/{id}"), never the raw URL, so
	// cardinality stays bounded regardless of ids in request paths.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Completed HTTP requests by method, route pattern and status code.",
		},
		[]string{"method", "path", "status"},
	)
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Request duration in seconds by method and route pattern; p95/p99 derive from these buckets.",
		},
		[]string{"method", "path"},
	)
	InFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Requests currently being served (probes excluded).",
		},
	)
)

// NewHandler returns the handler for the standalone metrics endpoint: only
// /metrics on a dedicated mux, so the second listener cannot expose any
// business route by accident.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	return mux
}
