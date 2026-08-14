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
	"ocm-backend/internal/xlsx"
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
	mux.Handle("GET /api/courses/export", read(h.exportCatalog))
	mux.Handle("GET /api/courses/{id}", read(h.getCatalog))
	mux.Handle("PUT /api/courses/{id}", manage(h.updateCatalog))
	mux.Handle("DELETE /api/courses/{id}", manage(h.deleteCatalog))

	// Course offerings (课程/开课)
	mux.Handle("GET /api/offerings", read(h.listOfferings))
	mux.Handle("POST /api/offerings", manage(h.createOffering))
	mux.Handle("GET /api/offerings/export", read(h.exportOfferings))
	mux.Handle("GET /api/offerings/{id}", read(h.getOffering))
	mux.Handle("PUT /api/offerings/{id}", manage(h.updateOffering))
	mux.Handle("DELETE /api/offerings/{id}", manage(h.deleteOffering))

	// Sessions (上课实例)
	mux.Handle("GET /api/sessions", read(h.listSessions))
	mux.Handle("POST /api/sessions", manage(h.createSession))
	mux.Handle("GET /api/sessions/export", read(h.exportSessions))
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

// exportCatalog streams the course catalog as an xlsx download. Columns match
// the importer's headers so the file round-trips.
func (h *Handler) exportCatalog(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListCatalog(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list courses")
		return
	}
	headers := []string{"name", "code", "credits", "total_hours", "category", "exam_type", "description"}
	rows := make([][]any, 0, len(list))
	for _, c := range list {
		rows = append(rows, []any{c.Name, c.Code, c.Credits, c.TotalHours, c.Category, c.ExamType, c.Description})
	}
	if err := xlsx.WriteExport(w, "courses.xlsx", "catalog", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export courses")
	}
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
	if errors.Is(err, ErrCodeTaken) {
		httpx.RespondError(w, http.StatusConflict, "course code already taken")
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
	if errors.Is(err, ErrCodeTaken) {
		httpx.RespondError(w, http.StatusConflict, "course code already taken")
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

// exportOfferings streams all offerings as an xlsx download, using display names
// (course name, teaching class name) so the file round-trips with the importer.
func (h *Handler) exportOfferings(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListOfferings(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list offerings")
		return
	}
	headers := []string{"course", "teaching_class", "semester", "teacher", "course_seq", "teacher_id", "teacher_title", "college", "max_students", "requirement", "weekly_hours", "note"}
	rows := make([][]any, 0, len(list))
	for _, o := range list {
		rows = append(rows, []any{o.CatalogName, o.TeachingClassName, o.Semester, o.Teacher, o.CourseSeq, o.TeacherID, o.TeacherTitle, o.College, o.MaxStudents, o.Requirement, o.WeeklyHours, o.Note})
	}
	if err := xlsx.WriteExport(w, "offerings.xlsx", "offerings", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export offerings")
	}
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
	list, err := h.querySessions(r)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list sessions")
		return
	}
	if list == nil {
		list = []SessionView{}
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

// querySessions parses the list/export filter params and returns the matching
// sessions, shared by listSessions and exportSessions so the two stay in sync.
func (h *Handler) querySessions(r *http.Request) ([]SessionView, error) {
	q := r.URL.Query()
	offeringID, _ := strconv.ParseInt(q.Get("offering_id"), 10, 64)
	classroomID, _ := strconv.ParseInt(q.Get("classroom_id"), 10, 64)
	return h.store.ListSessions(r.Context(), offeringID, classroomID, q.Get("from"), q.Get("to"))
}

// exportSessions streams sessions as an xlsx download, honoring the same
// filters (offering_id, classroom_id, from, to) as the list endpoint. Columns
// match the importer's headers (teacher included for human readability; the
// importer ignores unknown columns).
func (h *Handler) exportSessions(w http.ResponseWriter, r *http.Request) {
	list, err := h.querySessions(r)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list sessions")
		return
	}
	headers := []string{"date", "period_start", "period_end", "classroom", "course", "teaching_class", "semester", "teacher", "note"}
	rows := make([][]any, 0, len(list))
	for _, s := range list {
		rows = append(rows, []any{s.Date, s.PeriodStart, s.PeriodEnd, s.ClassroomName, s.CourseName, s.TeachingClassName, s.Semester, s.Teacher, s.Note})
	}
	if err := xlsx.WriteExport(w, "sessions.xlsx", "sessions", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export sessions")
	}
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
	in.Category = strings.TrimSpace(in.Category)
	in.ExamType = strings.TrimSpace(in.ExamType)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" {
		return "name is required", false
	}
	return "", true
}

// NormalizeCatalog is the exported wrapper around normalizeCatalog so the import
// framework reuses the same validation rules as the CRUD handlers.
func NormalizeCatalog(in *CatalogInput) (string, bool) {
	return normalizeCatalog(in)
}

func normalizeOffering(in *OfferingInput) (string, bool) {
	in.Teacher = strings.TrimSpace(in.Teacher)
	in.CourseSeq = strings.TrimSpace(in.CourseSeq)
	in.TeacherID = strings.TrimSpace(in.TeacherID)
	in.TeacherTitle = strings.TrimSpace(in.TeacherTitle)
	in.College = strings.TrimSpace(in.College)
	in.Requirement = strings.TrimSpace(in.Requirement)
	in.Semester = strings.TrimSpace(in.Semester)
	in.Note = strings.TrimSpace(in.Note)
	if in.CatalogID <= 0 {
		return "catalogId is required", false
	}
	if in.TeachingClassID <= 0 {
		return "teachingClassId is required", false
	}
	if in.Teacher == "" {
		return "teacher is required", false
	}
	if in.Semester == "" {
		return "semester is required", false
	}
	return "", true
}

// NormalizeOffering is the exported wrapper around normalizeOffering so the
// import framework reuses the same validation rules as the CRUD handlers.
func NormalizeOffering(in *OfferingInput) (string, bool) {
	return normalizeOffering(in)
}

// validateSession checks basic fields and that every period in
// [PeriodStart, PeriodEnd] exists in the active bell-time regime on the
// session's date, mirroring booking's validateBooking.
func (h *Handler) validateSession(ctx context.Context, in *SessionInput) (string, bool) {
	if in.OfferingID <= 0 {
		return "offeringId is required", false
	}
	if in.ClassroomID <= 0 {
		return "classroomId is required", false
	}
	if in.PeriodStart < 1 || in.PeriodEnd < 1 {
		return "periodStart and periodEnd must be >= 1", false
	}
	if in.PeriodStart > in.PeriodEnd {
		return "periodStart must be <= periodEnd", false
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
	valid := schedule.PeriodIndexSet(regime)
	for p := in.PeriodStart; p <= in.PeriodEnd; p++ {
		if !valid[p] {
			return "period range is not valid for the active regime on this date", false
		}
	}
	return "", true
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
