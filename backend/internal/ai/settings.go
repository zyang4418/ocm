package ai

import (
	"encoding/json"
	"net/http"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
)

// RegisterRoutes mounts the AI settings endpoints (admin-only) and the chat
// endpoint (permission-gated). The settings follow the same shape as the
// mail/storage configuration: viewable and editable only by the system admin
// role (the "*" wildcard), with the API key masked on read.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAdmin(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/settings/ai", wrap(h.getSettings))
	mux.Handle("PUT /api/settings/ai", wrap(h.putSettings))

	mux.Handle("POST /api/ai/chat",
		authenticate(authz.RequirePermission(authz.AiChat)(http.HandlerFunc(h.chat))))
}

// maskedSettings is the response shape: the API key is never returned —
// callers only learn whether one is set, so an admin editing the form leaves
// it blank to keep the stored value.
type maskedSettings struct {
	Enabled   bool   `json:"enabled"`
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	APIKeySet bool   `json:"apiKeySet"`
	Model     string `json:"model"`
}

func masked(s Settings) maskedSettings {
	return maskedSettings{
		Enabled:   s.Enabled,
		BaseURL:   s.BaseURL,
		APIKey:    "",
		APIKeySet: s.APIKey != "",
		Model:     s.Model,
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.Get(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not load ai settings", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, masked(settings))
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Normalize()
	// An empty API key means "keep the stored one"; the stored secret can
	// therefore never be cleared through the API. The merge runs before
	// validate so the required-when-enabled check sees the effective key.
	if in.APIKey == "" {
		existing, err := h.store.Get(r.Context())
		if err != nil {
			httpx.Error500(w, r, "could not load ai settings", err)
			return
		}
		in.APIKey = existing.APIKey
	}
	if msg, ok := validate(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.store.Update(r.Context(), in); err != nil {
		httpx.Error500(w, r, "could not update ai settings", err)
		return
	}
	// This request runs through the audit middleware, so the annotation lands
	// on its own audit row.
	systemlog.WithSummary(r.Context(), "修改AI助手配置")
	httpx.RespondJSON(w, http.StatusOK, masked(in))
}
