package httpx

import (
	"log"
	"net/http"
)

// Recover is an HTTP middleware that converts a handler panic into a logged
// 500 JSON response. Go's net/http already recovers request-goroutine panics,
// but only after closing the connection — the client sees a reset, not a JSON
// error. This returns a proper 500 so the frontend can show a message. It is
// defense-in-depth: it does not protect the importer's detached goroutines
// (processJob/runCommit recover themselves), only the synchronous request path.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("http: panic %s %s: %v", r.Method, r.URL.Path, rec)
				RespondError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
