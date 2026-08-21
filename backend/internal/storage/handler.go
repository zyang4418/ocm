package storage

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

// RegisterRoutes mounts the storage settings endpoints. Every route runs
// behind authenticate and the strict admin gate — the configuration is
// viewable and editable only by the system admin role (the "*" wildcard),
// never by roles granted ordinary permissions.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAdmin(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/settings/storage", wrap(h.getSettings))
	mux.Handle("PUT /api/settings/storage", wrap(h.putSettings))
}

// masked is the response shape: the secret key is never returned — callers
// only learn whether one is set, so an admin editing the form leaves it blank
// to keep the stored value.
type MaskedSettings struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	SecretKeySet  bool   `json:"secretKeySet"`
	UseSSL        bool   `json:"useSsl"`
	UsePathStyle  bool   `json:"usePathStyle"`
	PublicBaseURL string `json:"publicBaseUrl"`
}

func masked(s Settings) MaskedSettings {
	return MaskedSettings{
		Enabled:       s.Enabled,
		Endpoint:      s.Endpoint,
		Region:        s.Region,
		Bucket:        s.Bucket,
		AccessKey:     s.AccessKey,
		SecretKey:     "",
		SecretKeySet:  s.SecretKey != "",
		UseSSL:        s.UseSSL,
		UsePathStyle:  s.UsePathStyle,
		PublicBaseURL: s.PublicBaseURL,
	}
}

// @Summary      Get object storage settings (secret key masked)
// @Tags         settings
// @Produce      json
// @Success      200 {object} MaskedSettings "settings with the secret key hidden"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/settings/storage [get]
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.Get(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not load storage settings", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, masked(settings))
}

// @Summary      Update object storage settings
// @Description  An empty secretKey means "keep the stored one" — the secret
// @Description  can never be cleared through the API.
// @Tags         settings
// @Accept       json
// @Produce      json
// @Param        body body Settings true "storage settings"
// @Success      200 {object} MaskedSettings "updated settings (secret masked)"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / required fields missing when enabled"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/settings/storage [put]
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
	// An empty secret key means "keep the stored one"; the stored secret can
	// therefore never be cleared through the API.
	if in.SecretKey == "" {
		existing, err := h.store.Get(r.Context())
		if err != nil {
			httpx.Error500(w, r, "could not load storage settings", err)
			return
		}
		in.SecretKey = existing.SecretKey
	}
	if err := h.store.Update(r.Context(), in); err != nil {
		httpx.Error500(w, r, "could not update storage settings", err)
		return
	}
	// This request runs through the audit middleware, so the annotation lands
	// on its own audit row.
	systemlog.WithSummary(r.Context(), "修改对象存储配置")
	httpx.RespondJSON(w, http.StatusOK, masked(in))
}
