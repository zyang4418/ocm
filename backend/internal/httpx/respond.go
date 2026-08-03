package httpx

import (
	"encoding/json"
	"net/http"
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
