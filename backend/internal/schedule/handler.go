package schedule

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
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
	mux.Handle("GET /api/schedule/regimes/{id}", read(h.getRegime))
	mux.Handle("PUT /api/schedule/regimes/{id}", manage(h.updateRegime))
	mux.Handle("DELETE /api/schedule/regimes/{id}", manage(h.deleteRegime))
	mux.Handle("PUT /api/schedule/regimes/{id}/periods", manage(h.replacePeriods))
	mux.Handle("GET /api/schedule/active", read(h.activeRegime))
}

func (h *Handler) listRegimes(w http.ResponseWriter, r *http.Request) {
	regimes, err := h.store.ListRegimes(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list regimes")
		return
	}
	if regimes == nil {
		regimes = []Regime{}
	}
	httpx.RespondJSON(w, http.StatusOK, regimes)
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
		httpx.RespondError(w, http.StatusInternalServerError, "could not create regime")
		return
	}
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
		httpx.RespondError(w, http.StatusInternalServerError, "could not load regime")
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
		httpx.RespondError(w, http.StatusInternalServerError, "could not update regime")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, regime)
}

func (h *Handler) deleteRegime(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid regime id")
		return
	}
	if err := h.store.DeleteRegime(r.Context(), id); err != nil {
		if errors.Is(err, ErrRegimeNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "regime not found")
			return
		}
		httpx.RespondError(w, http.StatusInternalServerError, "could not delete regime")
		return
	}
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
		httpx.RespondError(w, http.StatusInternalServerError, "could not save periods")
		return
	}
	regime, err := h.store.GetRegime(r.Context(), id)
	if errors.Is(err, ErrRegimeNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "regime not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load regime")
		return
	}
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
		httpx.RespondError(w, http.StatusInternalServerError, "could not list regimes")
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
