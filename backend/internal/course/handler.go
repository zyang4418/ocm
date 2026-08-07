package course

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/schedule"
)

type Handler struct {
	store   *Store
	regimes *schedule.Store
}

func NewHandler(store *Store, regimes *schedule.Store) *Handler {
	return &Handler{store: store, regimes: regimes}
}

// RegisterRoutes mounts the catalog, offering, session and timetable
// endpoints. Reading requires course:read; management requires course:manage.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.CourseRead)(http.HandlerFunc(handler)))
	}
	manage := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.CourseManage)(http.HandlerFunc(handler)))
	}

	// Course catalog (课程库)
	mux.Handle("GET /api/courses", read(h.listCatalog))
	mux.Handle("POST /api/courses", manage(h.createCatalog))
	mux.Handle("GET /api/courses/{id}", read(h.getCatalog))
	mux.Handle("PUT /api/courses/{id}", manage(h.updateCatalog))
	mux.Handle("DELETE /api/courses/{id}", manage(h.deleteCatalog))

	// Course offerings (课程/开课)
	mux.Handle("GET /api/offerings", read(h.listOfferings))
	mux.Handle("POST /api/offerings", manage(h.createOffering))
	mux.Handle("GET /api/offerings/{id}", read(h.getOffering))
	mux.Handle("PUT /api/offerings/{id}", manage(h.updateOffering))
	mux.Handle("DELETE /api/offerings/{id}", manage(h.deleteOffering))

	// Sessions (上课实例)
	mux.Handle("GET /api/sessions", read(h.listSessions))
	mux.Handle("POST /api/sessions", manage(h.createSession))
	mux.Handle("GET /api/sessions/{id}", read(h.getSession))
	mux.Handle("PUT /api/sessions/{id}", manage(h.updateSession))
	mux.Handle("DELETE /api/sessions/{id}", manage(h.deleteSession))

	// Classroom timetable (教室课表)
	mux.Handle("GET /api/timetable", read(h.timetable))
}

// ---- Catalog ----

func (h *Handler) listCatalog(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListCatalog(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list courses")
		return
	}
	if list == nil {
		list = []CatalogCourse{}
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

func (h *Handler) createCatalog(w http.ResponseWriter, r *http.Request) {
	var in CatalogInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeCatalog(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := h.store.CreateCatalog(r.Context(), in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "course name already taken")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create course")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, c)
}

func (h *Handler) getCatalog(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid course id")
		return
	}
	c, err := h.store.GetCatalog(r.Context(), id)
	if errors.Is(err, ErrCatalogNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "course not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load course")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

func (h *Handler) updateCatalog(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid course id")
		return
	}
	var in CatalogInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeCatalog(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := h.store.UpdateCatalog(r.Context(), id, in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "course name already taken")
		return
	}
	if errors.Is(err, ErrCatalogNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "course not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update course")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

func (h *Handler) deleteCatalog(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid course id")
		return
	}
	if err := h.store.DeleteCatalog(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, ErrInUse):
			httpx.RespondError(w, http.StatusConflict, "course is referenced by offerings and cannot be deleted")
		case errors.Is(err, ErrCatalogNotFound):
			httpx.RespondError(w, http.StatusNotFound, "course not found")
		default:
			httpx.RespondError(w, http.StatusInternalServerError, "could not delete course")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Offerings ----

func (h *Handler) listOfferings(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListOfferings(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list offerings")
		return
	}
	if list == nil {
		list = []OfferingView{}
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

func (h *Handler) createOffering(w http.ResponseWriter, r *http.Request) {
	var in OfferingInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeOffering(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	v, err := h.store.CreateOffering(r.Context(), in)
	if errors.Is(err, ErrOfferingTaken) {
		httpx.RespondError(w, http.StatusConflict, "this course offering already exists")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create offering")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, v)
}

func (h *Handler) getOffering(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid offering id")
		return
	}
	v, err := h.store.GetOffering(r.Context(), id)
	if errors.Is(err, ErrOfferingNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "offering not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load offering")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) updateOffering(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid offering id")
		return
	}
	var in OfferingInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeOffering(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	v, err := h.store.UpdateOffering(r.Context(), id, in)
	if errors.Is(err, ErrOfferingTaken) {
		httpx.RespondError(w, http.StatusConflict, "this course offering already exists")
		return
	}
	if errors.Is(err, ErrOfferingNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "offering not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update offering")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) deleteOffering(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid offering id")
		return
	}
	if err := h.store.DeleteOffering(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, ErrInUse):
			httpx.RespondError(w, http.StatusConflict, "offering has sessions and cannot be deleted")
		case errors.Is(err, ErrOfferingNotFound):
			httpx.RespondError(w, http.StatusNotFound, "offering not found")
		default:
			httpx.RespondError(w, http.StatusInternalServerError, "could not delete offering")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Sessions ----

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offeringID, _ := strconv.ParseInt(q.Get("offering_id"), 10, 64)
	classroomID, _ := strconv.ParseInt(q.Get("classroom_id"), 10, 64)
	list, err := h.store.ListSessions(r.Context(), offeringID, classroomID, q.Get("from"), q.Get("to"))
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list sessions")
		return
	}
	if list == nil {
		list = []SessionView{}
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var in SessionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := h.validateSession(r.Context(), &in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	v, err := h.store.CreateSession(r.Context(), in)
	if errors.Is(err, ErrClassroomConflict) {
		httpx.RespondError(w, http.StatusConflict, "classroom already booked for this date and period")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, v)
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	v, err := h.store.GetSession(r.Context(), id)
	if errors.Is(err, ErrSessionNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load session")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) updateSession(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	var in SessionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := h.validateSession(r.Context(), &in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	v, err := h.store.UpdateSession(r.Context(), id, in)
	if errors.Is(err, ErrClassroomConflict) {
		httpx.RespondError(w, http.StatusConflict, "classroom already booked for this date and period")
		return
	}
	if errors.Is(err, ErrSessionNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update session")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if err := h.store.DeleteSession(r.Context(), id); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "session not found")
			return
		}
		httpx.RespondError(w, http.StatusInternalServerError, "could not delete session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Timetable ----

func (h *Handler) timetable(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	classroomID, err := strconv.ParseInt(q.Get("classroom_id"), 10, 64)
	if err != nil || classroomID <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "classroom_id is required")
		return
	}
	from := q.Get("from")
	to := q.Get("to")
	if from == "" || to == "" {
		httpx.RespondError(w, http.StatusBadRequest, "from and to dates are required (YYYY-MM-DD)")
		return
	}
	days, err := h.store.Timetable(r.Context(), classroomID, from, to, h.regimes)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not build timetable")
		return
	}
	if days == nil {
		days = []TimetableDay{}
	}
	httpx.RespondJSON(w, http.StatusOK, days)
}

// ---- validation ----

func normalizeCatalog(in *CatalogInput) (string, bool) {
	in.Name = strings.TrimSpace(in.Name)
	in.Code = strings.TrimSpace(in.Code)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" {
		return "name is required", false
	}
	return "", true
}

func normalizeOffering(in *OfferingInput) (string, bool) {
	in.ClassName = strings.TrimSpace(in.ClassName)
	in.Teacher = strings.TrimSpace(in.Teacher)
	in.Semester = strings.TrimSpace(in.Semester)
	in.Note = strings.TrimSpace(in.Note)
	if in.CatalogID <= 0 {
		return "catalogId is required", false
	}
	if in.ClassName == "" {
		return "className is required", false
	}
	if in.Semester == "" {
		return "semester is required", false
	}
	return "", true
}

// validateSession checks basic fields and that periodIndex is valid for the
// active bell-time regime on the session's date.
func (h *Handler) validateSession(ctx context.Context, in *SessionInput) (string, bool) {
	if in.OfferingID <= 0 {
		return "offeringId is required", false
	}
	if in.ClassroomID <= 0 {
		return "classroomId is required", false
	}
	if in.PeriodIndex < 1 {
		return "periodIndex must be >= 1", false
	}
	date, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		return "invalid date (use YYYY-MM-DD)", false
	}
	in.Note = strings.TrimSpace(in.Note)
	regimes, err := h.regimes.ListRegimes(ctx)
	if err != nil {
		return "could not validate against schedule regime", false
	}
	regime, ok := schedule.ActiveFor(regimes, date)
	if !ok {
		return "no schedule regime configured for this date", false
	}
	if !schedule.PeriodIndexSet(regime)[in.PeriodIndex] {
		return "periodIndex is not valid for the active regime on this date", false
	}
	return "", true
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
