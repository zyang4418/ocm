package mail

import (
	"encoding/json"
	"net/http"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

// RegisterRoutes mounts the mail settings endpoints. Every route runs behind
// authenticate and the strict admin gate — the configuration is viewable and
// editable only by the system admin role (the "*" wildcard), never by roles
// granted ordinary permissions.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAdmin(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/settings/email", wrap(h.getSettings))
	mux.Handle("PUT /api/settings/email", wrap(h.putSettings))
}

// masked is the response shape: the password is never returned — callers only
// learn whether one is set, so an admin editing the form leaves it blank to
// keep the stored value.
type maskedSettings struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	PasswordSet bool   `json:"passwordSet"`
	FromName    string `json:"fromName"`
	FromAddress string `json:"fromAddress"`
	Encryption  string `json:"encryption"`
}

func masked(s Settings) maskedSettings {
	return maskedSettings{
		Enabled:     s.Enabled,
		Host:        s.Host,
		Port:        s.Port,
		Username:    s.Username,
		Password:    "",
		PasswordSet: s.Password != "",
		FromName:    s.FromName,
		FromAddress: s.FromAddress,
		Encryption:  s.Encryption,
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.Get(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not load mail settings", err)
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
	if msg, ok := validate(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	// An empty password means "keep the stored one"; the stored secret can
	// therefore never be cleared through the API.
	if in.Password == "" {
		existing, err := h.store.Get(r.Context())
		if err != nil {
			httpx.Error500(w, r, "could not load mail settings", err)
			return
		}
		in.Password = existing.Password
	}
	if err := h.store.Update(r.Context(), in); err != nil {
		httpx.Error500(w, r, "could not update mail settings", err)
		return
	}
	// This request runs through the audit middleware, so the annotation lands
	// on its own audit row.
	systemlog.WithSummary(r.Context(), "修改邮件服务配置")
	httpx.RespondJSON(w, http.StatusOK, masked(in))
}
