package httpx

import (
	"encoding/json"
	"net/http"

	"ocm-backend/internal/logging"
)

// RespondJSON writes v as JSON with the given HTTP status code.
func RespondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// RespondError writes a JSON error body {"error": message} with the given status.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}

// Error500 logs the underlying error for operators — handlers respond with a
// generic message so the raw error would otherwise be discarded — then
// responds 500 with that message. errs is optional: some call sites have no
// error value in scope. The request_id ties the line to the access-log line
// of the same request.
func Error500(w http.ResponseWriter, r *http.Request, message string, errs ...error) {
	attrs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"request_id", logging.RequestIDFrom(r.Context()),
	}
	if len(errs) > 0 && errs[0] != nil {
		attrs = append(attrs, "err", errs[0])
	}
	logging.L.Error("http: internal error: "+message, attrs...)
	RespondError(w, http.StatusInternalServerError, message)
}
