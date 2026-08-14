package httpx

import (
	"net"
	"net/http"
)

// StatusRecorder wraps a ResponseWriter to capture the response status code
// for post-request logging (access log, audit log). Nested recorders propagate
// the status up the chain through the underlying writer.
type StatusRecorder struct {
	http.ResponseWriter
	code int
}

func NewStatusRecorder(w http.ResponseWriter) *StatusRecorder {
	return &StatusRecorder{ResponseWriter: w}
}

func (r *StatusRecorder) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *StatusRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Code reports the recorded status code (0 if nothing was written yet).
func (r *StatusRecorder) Code() int { return r.code }

// Flush forwards to the underlying writer when it supports it (the xlsx
// export streams its response).
func (r *StatusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ClientIP extracts the remote address host from r.RemoteAddr. The
// conservative choice: X-Forwarded-For is client-spoofable and the app has no
// proxy-trust configuration. A future trusted-edge deployment may swap this
// for the first X-Forwarded-For entry behind an explicit trust flag.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
