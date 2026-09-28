package metrics

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestObserveSQLDBGauges(t *testing.T) {
	// sql.Open is lazy: no dial happens, so Stats() answers without a server.
	db, err := sql.Open("mysql", "u:p@/d")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ObserveSQLDB(db)

	body := scrape(t)
	for _, name := range []string{
		"ocm_db_connections_open",
		"ocm_db_connections_idle",
		"ocm_db_connections_in_use",
		"ocm_db_max_open_connections",
		"ocm_db_wait_count_total",
		"ocm_db_wait_duration_seconds_total",
	} {
		if !containsMetric(body, name) {
			t.Errorf("metrics output missing %s", name)
		}
	}
}

func TestMetricsHandlerOnlyServesMetrics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/bookings", nil)
	rec := httptest.NewRecorder()
	NewHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("metrics mux status for business path = %d, want 404", rec.Code)
	}
}

func scrape(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	NewHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

// containsMetric matches a non-comment exposition line whose metric name is
// exactly name (name followed by a label set or a value).
func containsMetric(body, name string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimLeft(line, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, name+"{") || strings.HasPrefix(line, name+" ") {
			return true
		}
	}
	return false
}
