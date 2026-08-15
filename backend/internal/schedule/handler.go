package schedule

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
	"ocm-backend/internal/xlsx"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts the regime/period endpoints. Reading (list/get/active)
// requires course:read; management (create/update/delete/replace-periods)
// requires course:manage.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.CourseRead)(http.HandlerFunc(handler)))
	}
	manage := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.CourseManage)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/schedule/regimes", read(h.listRegimes))
	mux.Handle("POST /api/schedule/regimes", manage(h.createRegime))
	mux.Handle("GET /api/schedule/regimes/export", read(h.exportRegimes))
	mux.Handle("GET /api/schedule/regimes/{id}", read(h.getRegime))
	mux.Handle("PUT /api/schedule/regimes/{id}", manage(h.updateRegime))
	mux.Handle("DELETE /api/schedule/regimes/{id}", manage(h.deleteRegime))
	mux.Handle("PUT /api/schedule/regimes/{id}/periods", manage(h.replacePeriods))
	mux.Handle("GET /api/schedule/active", read(h.activeRegime))
}

func (h *Handler) listRegimes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	regimes, total, err := h.store.PageRegimes(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list regimes", err)
		return
	}
	if regimes == nil {
		regimes = []Regime{}
	}
	httpx.RespondPaged(w, regimes, total, p)
}

// exportRegimes streams all regimes as an xlsx download, flattened to one row
// per period (regime columns repeated). The layout round-trips with the
// importer, which groups by regime_name and replaces the period set.
func (h *Handler) exportRegimes(w http.ResponseWriter, r *http.Request) {
	regimes, err := h.store.ListRegimes(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list regimes", err)
		return
	}
	headers := []string{"regime_name", "effective_month", "effective_day", "period_index", "start_time", "end_time"}
	rows := make([][]any, 0)
	for _, rg := range regimes {
		for _, p := range rg.Periods {
			rows = append(rows, []any{rg.Name, rg.EffectiveMonth, rg.EffectiveDay, p.PeriodIndex, p.StartTime, p.EndTime})
		}
	}
	if err := xlsx.WriteExport(w, "regimes.xlsx", "regimes", headers, rows); err != nil {
		httpx.Error500(w, r, "could not export regimes", err)
	}
}

func (h *Handler) createRegime(w http.ResponseWriter, r *http.Request) {
	var in RegimeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeRegime(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	regime, err := h.store.CreateRegime(r.Context(), in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "regime name already taken")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not create regime", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建作息制度 %s", regime.Name))
	httpx.RespondJSON(w, http.StatusCreated, regime)
}

func (h *Handler) getRegime(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid regime id")
		return
	}
	regime, err := h.store.GetRegime(r.Context(), id)
	if errors.Is(err, ErrRegimeNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "regime not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load regime", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, regime)
}

func (h *Handler) updateRegime(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid regime id")
		return
	}
	var in RegimeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeRegime(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	regime, err := h.store.UpdateRegime(r.Context(), id, in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "regime name already taken")
		return
	}
	if errors.Is(err, ErrRegimeNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "regime not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update regime", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新作息制度 %s", regime.Name))
	httpx.RespondJSON(w, http.StatusOK, regime)
}

func (h *Handler) deleteRegime(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid regime id")
		return
	}
	existing, err := h.store.GetRegime(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrRegimeNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "regime not found")
			return
		}
		httpx.Error500(w, r, "could not load regime", err)
		return
	}
	if err := h.store.DeleteRegime(r.Context(), id); err != nil {
		if errors.Is(err, ErrRegimeNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "regime not found")
			return
		}
		httpx.Error500(w, r, "could not delete regime", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除作息制度 %s", existing.Name))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) replacePeriods(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid regime id")
		return
	}
	var in struct {
		Periods []PeriodInput `json:"periods"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizePeriods(in.Periods); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.store.ReplacePeriods(r.Context(), id, in.Periods); err != nil {
		httpx.Error500(w, r, "could not save periods", err)
		return
	}
	regime, err := h.store.GetRegime(r.Context(), id)
	if errors.Is(err, ErrRegimeNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "regime not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load regime", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("调整作息制度 %s 节次", regime.Name))
	httpx.RespondJSON(w, http.StatusOK, regime)
}

func (h *Handler) activeRegime(w http.ResponseWriter, r *http.Request) {
	date := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		parsed, err := time.Parse("2006-01-02", v)
		if err != nil {
			httpx.RespondError(w, http.StatusBadRequest, "invalid date (use YYYY-MM-DD)")
			return
		}
		date = parsed
	}
	regimes, err := h.store.ListRegimes(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list regimes", err)
		return
	}
	regime, ok := ActiveFor(regimes, date)
	if !ok {
		httpx.RespondError(w, http.StatusNotFound, "no schedule regime configured")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, regime)
}

func normalizeRegime(in *RegimeInput) (string, bool) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "name is required", false
	}
	if in.EffectiveMonth < 1 || in.EffectiveMonth > 12 {
		return "effectiveMonth must be 1-12", false
	}
	if in.EffectiveDay < 1 || in.EffectiveDay > 31 {
		return "effectiveDay must be 1-31", false
	}
	return "", true
}

// NormalizeRegime is the exported wrapper around normalizeRegime so the import
// framework reuses the same validation rules as the CRUD handlers.
func NormalizeRegime(in *RegimeInput) (string, bool) {
	return normalizeRegime(in)
}

// normalizePeriods validates a period set: each period has a positive unique
// index and a valid time range (end after start).
func normalizePeriods(periods []PeriodInput) (string, bool) {
	seen := make(map[int]bool, len(periods))
	for _, p := range periods {
		if p.PeriodIndex < 1 {
			return "period index must be >= 1", false
		}
		if seen[p.PeriodIndex] {
			return "duplicate period index", false
		}
		seen[p.PeriodIndex] = true
		start, ok := parseTime(p.StartTime)
		if !ok {
			return "invalid start time (use HH:MM)", false
		}
		end, ok := parseTime(p.EndTime)
		if !ok {
			return "invalid end time (use HH:MM)", false
		}
		if !end.After(start) {
			return "end time must be after start time", false
		}
	}
	return "", true
}

// NormalizePeriods is the exported wrapper around normalizePeriods so the import
// framework reuses the same validation rules as the CRUD handlers.
func NormalizePeriods(periods []PeriodInput) (string, bool) {
	return normalizePeriods(periods)
}

func parseTime(s string) (time.Time, bool) {
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
