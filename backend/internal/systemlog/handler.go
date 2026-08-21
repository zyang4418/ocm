package systemlog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

// RegisterRoutes mounts the log list and retention-settings endpoints. Every
// route runs behind authenticate and a permission check.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(perm string, handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(perm)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/logs", wrap(authz.LogRead, h.list))
	mux.Handle("GET /api/logs/settings", wrap(authz.LogRead, h.getSettings))
	mux.Handle("PUT /api/logs/settings", wrap(authz.LogManage, h.putSettings))
}

// parseDay validates a YYYY-MM-DD filter value (same rule as the booking
// from/to filter).
func parseDay(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return "", false
	}
	return raw, true
}

// @Summary      List audit log entries
// @Tags         logs
// @Produce      json
// @Param        from query string false "range start (Y-M-D)"
// @Param        to query string false "range end (Y-M-D)"
// @Param        q query string false "search"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged log views"
// @Failure      400 {object} httpx.ErrorResponse "invalid date filter"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/logs [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, ok := parseDay(q.Get("from"))
	if !ok {
		httpx.RespondError(w, http.StatusBadRequest, "invalid from date, expected YYYY-MM-DD")
		return
	}
	to, ok := parseDay(q.Get("to"))
	if !ok {
		httpx.RespondError(w, http.StatusBadRequest, "invalid to date, expected YYYY-MM-DD")
		return
	}
	p := httpx.ParsePageParams(q)
	items, total, err := h.store.PageLogs(r.Context(), LogFilter{From: from, To: to},
		httpx.ParseSearch(q), dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list logs", err)
		return
	}
	httpx.RespondPaged(w, items, total, p)
}

// @Summary      Get log retention settings
// @Tags         logs
// @Produce      json
// @Success      200 {object} Settings "retention settings"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/logs/settings [get]
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not load log settings", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, settings)
}

// @Summary      Update log retention settings
// @Tags         logs
// @Accept       json
// @Produce      json
// @Param        body body Settings true "retention settings"
// @Success      200 {object} Settings "updated retention settings"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / retentionDays out of range"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/logs/settings [put]
func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.RetentionDays < 1 || in.RetentionDays > MaxRetentionDays {
		httpx.RespondError(w, http.StatusBadRequest,
			fmt.Sprintf("retentionDays must be between 1 and %d", MaxRetentionDays))
		return
	}
	if err := h.store.UpdateSettings(r.Context(), in); err != nil {
		httpx.Error500(w, r, "could not update log settings", err)
		return
	}
	state := "关闭"
	if in.RetentionEnabled {
		state = "开启"
	}
	// This request runs through the audit middleware, so the annotation lands
	// on its own audit row.
	WithSummary(r.Context(), fmt.Sprintf("修改日志保留设置：%s，保留 %d 天", state, in.RetentionDays))
	httpx.RespondJSON(w, http.StatusOK, in)
}
