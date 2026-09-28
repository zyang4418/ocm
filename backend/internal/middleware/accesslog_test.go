package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccessLogSetsRequestIDHeader(t *testing.T) {
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	ids := make(map[string]bool)
	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/thing", nil))
		id := rec.Header().Get("X-Request-ID")
		if id == "" {
			t.Fatal("X-Request-ID header missing on error response")
		}
		if ids[id] {
			t.Fatalf("request id %q repeated across requests", id)
		}
		ids[id] = true
	}
}
