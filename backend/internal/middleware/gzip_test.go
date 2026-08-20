package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ocm-backend/internal/httpx"
)

// serve runs h against a fresh recorder wrapped in a StatusRecorder, mirroring
// the production stack (AccessLog wraps the real writer in a StatusRecorder,
// then Recover, then Gzip sit inside it). Returns the recorder (for headers /
// body) and the recorder's StatusRecorder (for the recorded status).
func serve(t *testing.T, h http.Handler, method, target string, acceptGzip bool) (*httptest.ResponseRecorder, *httpx.StatusRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	sr := httpx.NewStatusRecorder(rec)
	req := httptest.NewRequest(method, target, nil)
	if acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	h.ServeHTTP(sr, req)
	return rec, sr
}

func TestShouldGzip(t *testing.T) {
	cases := map[string]bool{
		"application/json":                         true,
		"application/json; charset=utf-8":           true,
		"APPLICATION/JSON":                          true,
		"text/event-stream; charset=utf-8":         false,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": false,
		"text/plain; charset=utf-8":                 false,
		"":  false,
	}
	for ct, want := range cases {
		if got := shouldGzip(ct); got != want {
			t.Errorf("shouldGzip(%q) = %v, want %v", ct, got, want)
		}
	}
}

func TestClientAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"gzip":              true,
		"gzip, deflate, br": true,
		"deflate, br":       false,
		"identity":          false,
		"x-gzip":            false, // not the gzip token
		"gzip;q=0.8":        true,
	}
	for ae, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if ae != "" {
			req.Header.Set("Accept-Encoding", ae)
		}
		if got := clientAcceptsGzip(req); got != want {
			t.Errorf("clientAcceptsGzip(%q) = %v, want %v", ae, got, want)
		}
	}
}

func TestGzipCompressesJSON(t *testing.T) {
	body := []byte(`{"hello":"world","items":[1,2,3,4,5,6,7,8,9,10]}`)
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	rec, sr := serve(t, h, http.MethodGet, "/", true)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if sr.Code() != http.StatusOK {
		t.Fatalf("recorded status = %d, want 200", sr.Code())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v (body was not a valid gzip stream)", err)
	}
	dec, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	if !bytes.Equal(dec, body) {
		t.Fatalf("decompressed body = %q, want %q", dec, body)
	}
	if rec.Body.Len() >= len(body) {
		t.Fatalf("compressed size %d >= raw size %d (gzip should shrink JSON)", rec.Body.Len(), len(body))
	}
}

func TestGzipPassesThroughWithoutAcceptEncoding(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	rec, _ := serve(t, h, http.MethodGet, "/", false)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (no Accept-Encoding)", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("body = %q, want raw %q", rec.Body.Bytes(), body)
	}
}

func TestGzipSkipsSSE(t *testing.T) {
	frame := []byte("event: msg\ndata: {}\n\n")
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(frame)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	rec, sr := serve(t, h, http.MethodPost, "/api/ai/chat", true)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (SSE must stream raw)", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), frame) {
		t.Fatalf("body = %q, want raw frame %q", rec.Body.Bytes(), frame)
	}
	if sr.Code() != http.StatusOK {
		t.Fatalf("recorded status = %d, want 200", sr.Code())
	}
}

func TestGzipSkipsNoContent(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	}))
	rec, sr := serve(t, h, http.MethodDelete, "/api/x/1", true)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty for 204", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty for 204", rec.Body.Bytes())
	}
	if sr.Code() != http.StatusNoContent {
		t.Fatalf("recorded status = %d, want 204", sr.Code())
	}
}

func TestGzipPanic500BypassesCompression(t *testing.T) {
	// Recover(Gzip(handler)) where the handler panics before writing. The
	// recovered 500 is written to the StatusRecorder Recover received — not
	// the gzip writer — so it must be a clean, uncompressed JSON error.
	stack := httpx.Recover(Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		panic("boom")
	})))
	rec, sr := serve(t, stack, http.MethodGet, "/api/x", true)

	if sr.Code() != http.StatusInternalServerError {
		t.Fatalf("recorded status = %d, want 500", sr.Code())
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (panic 500 must bypass gzip)", got)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"error"`)) {
		t.Fatalf("body = %q, want JSON containing \"error\"", rec.Body.Bytes())
	}
	// And it must not be a gzip stream (no valid gzip header).
	if _, err := gzip.NewReader(rec.Body); err == nil {
		t.Fatalf("body decoded as gzip; panic 500 should be raw JSON")
	}
}
