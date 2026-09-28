package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// noProxyClient sidesteps any system proxy: httptest servers are loopback
// only and must be reached directly.
var noProxyClient = &http.Client{Transport: &http.Transport{}}

// newTestStack builds a ServeMux with method-routed patterns (the production
// registration style) so routePattern sees real r.Pattern values, wrapped in
// the metrics middleware exactly as main.go composes it.
func newTestStack() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/things/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(Middleware(mux))
}

func TestMiddlewareLabelsByRoutePattern(t *testing.T) {
	ts := newTestStack()
	defer ts.Close()

	resp, err := noProxyClient.Get(ts.URL + "/api/things/7")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("GET", "/api/things/{id}", "200")); got != 1 {
		t.Fatalf("pattern-labelled counter = %v, want 1 (raw path /api/things/7 must never appear)", got)
	}
	if testutil.CollectAndCount(RequestDuration, "http_request_duration_seconds") == 0 {
		t.Fatal("duration histogram recorded no sample")
	}
}

func TestMiddlewareUnmatched(t *testing.T) {
	ts := newTestStack()
	defer ts.Close()

	resp, err := noProxyClient.Get(ts.URL + "/api/none")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("GET", reportUnmatched, "404")); got != 1 {
		t.Fatalf("unmatched counter = %v, want 1", got)
	}
}

func TestMiddlewareSkipsProbes(t *testing.T) {
	ts := newTestStack()
	defer ts.Close()

	for _, probe := range []string{"/healthz", "/readyz"} {
		resp, err := noProxyClient.Get(ts.URL + probe)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	// WithLabelValues materialises the series at zero if it was never
	// incremented, so a zero value proves the probe was skipped.
	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("GET", "/healthz", "200")); got != 0 {
		t.Fatalf("probe counter = %v, want 0 (probes must not be instrumented)", got)
	}
}

func TestMiddlewareInFlight(t *testing.T) {
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/slow", func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(Middleware(mux))
	defer ts.Close()

	done := make(chan struct{})
	go func() {
		resp, err := noProxyClient.Get(ts.URL + "/api/slow")
		if err == nil {
			_ = resp.Body.Close()
		}
		close(done)
	}()

	waitInFlight(t, 1)
	close(release)
	<-done
	waitInFlight(t, 0)
}

func waitInFlight(t *testing.T, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for testutil.ToFloat64(InFlight) != want {
		if time.Now().After(deadline) {
			t.Fatalf("in-flight gauge stuck at %v, want %v", testutil.ToFloat64(InFlight), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
