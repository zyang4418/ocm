package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// Gzip transparently compresses application/json responses when the client
// advertises Accept-Encoding: gzip. It is mounted as the INNERMOST layer of
// the global stack (AccessLog(Recover(Gzip(mux)))) so that:
//
//   - AccessLog stays outermost and still records the real status (including
//     recovered-panic 500s).
//   - A handler panic unwinds past Gzip (its defer only flushes if a gzip
//     writer was engaged) to Recover, which writes the 500 directly to the
//     StatusRecorder it received — bypassing Gzip entirely, so the error body
//     is a clean uncompressed JSON 500, not a half-started gzip stream.
//
// Engagement is lazy and content-typed: the gzip writer is created only when
// the response's Content-Type is application/json (set by httpx.RespondJSON
// before WriteHeader) and the status permits a body. SSE
// (text/event-stream) and xlsx exports (application/vnd...sheet) match neither
// rule and pass through uncompressed — the former so its per-frame Flush stays
// unbuffered, the latter because xlsx is already a zip. 204/304 responses have
// no body and are skipped. Responses to clients that do not accept gzip are
// passed through untouched (no wrapper, no overhead). There is no minimum-size
// gate, so very small JSON acks grow slightly from the gzip header; on this
// system that is negligible, and the responses that motivated this (list /
// rows / detail) are KB–MB.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !clientAcceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		g := &gzipResponseWriter{ResponseWriter: w}
		defer g.close()
		next.ServeHTTP(g, r)
	})
}

// gzipResponseWriter wraps the underlying ResponseWriter (the StatusRecorder in
// the global stack) and compresses the body once engaged. Header() is
// inherited from the embedded writer, so Content-Type / Content-Encoding set
// here land on the real connection's header map before it is flushed.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz            *gzip.Writer
	enabled       bool // a gzip writer is active for this response
	headerWritten bool // WriteHeader has been called (explicit status committed)
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.headerWritten = true
	g.maybeEngage(code)
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	// If the handler writes without calling WriteHeader, net/http implicitly
	// flushes a 200 on the first Write; decide compression now (before that
	// flush) so Content-Encoding is on the wire. If WriteHeader already ran,
	// the decision is final — don't re-decide mid-stream.
	if !g.enabled && !g.headerWritten {
		g.maybeEngage(http.StatusOK)
	}
	if g.enabled {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer so SSE (which passes through
// unengaged) and any flushed handler keep draining. When a gzip writer is
// active it is flushed first so compressed bytes reach the client promptly.
func (g *gzipResponseWriter) Flush() {
	if g.enabled && g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponseWriter) close() {
	if g.enabled && g.gz != nil {
		_ = g.gz.Close()
	}
}

// maybeEngage decides, at most once, whether to gzip this response. It must run
// before the real WriteHeader flushes, so it sets Content-Encoding and drops
// Content-Length in the header map first. Idempotent: once enabled (or once
// skipped because the client doesn't accept gzip) it does nothing on retry.
func (g *gzipResponseWriter) maybeEngage(code int) {
	if g.enabled {
		return
	}
	if code == http.StatusNoContent || code == http.StatusNotModified {
		return
	}
	if !shouldGzip(g.Header().Get("Content-Type")) {
		return
	}
	g.Header().Set("Content-Encoding", "gzip")
	g.Header().Del("Content-Length")
	g.gz = gzip.NewWriter(g.ResponseWriter)
	g.enabled = true
}

// shouldGzip reports whether a response Content-Type is worth gzipping. Only
// application/json is allowed: that covers every large response (lists, detail,
// rows, dashboard) while excluding text/event-stream (SSE — must stream) and
// the xlsx mime (already zip-compressed). A trailing ;charset=... is stripped.
func shouldGzip(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct == "application/json"
}

// clientAcceptsGzip parses Accept-Encoding for a gzip token. It ignores q
// values (no client in practice refuses the one encoding it advertises).
func clientAcceptsGzip(r *http.Request) bool {
	enc := r.Header.Get("Accept-Encoding")
	if enc == "" {
		return false
	}
	for _, tok := range strings.Split(enc, ",") {
		name, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tok)), ";")
		if name == "gzip" {
			return true
		}
	}
	return false
}
