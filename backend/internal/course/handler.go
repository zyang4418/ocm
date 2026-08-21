package course

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/schedule"
	"ocm-backend/internal/systemlog"
	"ocm-backend/internal/xlsx"
)

type Handler struct {
	store      *Store
	classrooms *classroom.Store
	regimes    *schedule.Store
}

func NewHandler(store *Store, classrooms *classroom.Store, regimes *schedule.Store) *Handler {
	return &Handler{store: store, classrooms: classrooms, regimes: regimes}
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
	mux.Handle("GET /api/timetable/export", read(h.timetableExport))
}

// ---- Catalog ----

// @Summary      List course catalog entries
// @Tags         courses
// @Produce      json
// @Param        q query string false "search by code/name"
// @Param        department query string false "filter by department"
// @Param        credit query number false "filter by credit"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged catalog entries"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/courses [get]
func (h *Handler) listCatalog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	list, total, err := h.store.PageCatalog(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list courses", err)
		return
	}
	if list == nil {
		list = []CatalogCourse{}
	}
	httpx.RespondPaged(w, list, total, p)
}

// exportCatalog streams the course catalog as an xlsx download. Columns match
// the importer's headers so the file round-trips.
func (h *Handler) exportCatalog(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListCatalog(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list courses", err)
		return
	}
	headers := []string{"name", "code", "credits", "total_hours", "category", "exam_type", "description"}
	rows := make([][]any, 0, len(list))
	for _, c := range list {
		rows = append(rows, []any{c.Name, c.Code, c.Credits, c.TotalHours, c.Category, c.ExamType, c.Description})
	}
	if err := xlsx.WriteExport(w, "courses.xlsx", "catalog", headers, rows); err != nil {
		httpx.Error500(w, r, "could not export courses", err)
	}
}

// @Summary      Create a course catalog entry
// @Tags         courses
// @Accept       json
// @Produce      json
// @Param        body body CatalogInput true "catalog input"
// @Success      201 {object} CatalogCourse "created catalog entry"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / required fields missing"
// @Failure      409 {object} httpx.ErrorResponse "catalog code already taken"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/courses [post]
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
		httpx.Error500(w, r, "could not create course", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建课程 %s", c.Name))
	httpx.RespondJSON(w, http.StatusCreated, c)
}

// @Summary      Get a course catalog entry
// @Tags         courses
// @Produce      json
// @Param        id path int true "catalog id"
// @Success      200 {object} CatalogCourse "catalog detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid catalog id"
// @Failure      404 {object} httpx.ErrorResponse "catalog entry not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/courses/{id} [get]
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
		httpx.Error500(w, r, "could not load course", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

// @Summary      Update a course catalog entry
// @Tags         courses
// @Accept       json
// @Produce      json
// @Param        id path int true "catalog id"
// @Param        body body CatalogInput true "catalog input"
// @Success      200 {object} CatalogCourse "updated catalog entry"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / required fields missing"
// @Failure      404 {object} httpx.ErrorResponse "catalog entry not found"
// @Failure      409 {object} httpx.ErrorResponse "catalog code already taken"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/courses/{id} [put]
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
		httpx.Error500(w, r, "could not update course", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新课程 %s", c.Name))
	httpx.RespondJSON(w, http.StatusOK, c)
}

// @Summary      Delete a course catalog entry
// @Tags         courses
// @Param        id path int true "catalog id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid catalog id"
// @Failure      404 {object} httpx.ErrorResponse "catalog entry not found"
// @Failure      409 {object} httpx.ErrorResponse "offerings still reference the catalog"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/courses/{id} [delete]
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
			httpx.Error500(w, r, "could not delete course", err)
		}
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除课程 #%d", id))
	w.WriteHeader(http.StatusNoContent)
}

// ---- Offerings ----

// @Summary      List offerings
// @Tags         offerings
// @Produce      json
// @Param        semester query string false "filter by semester"
// @Param        catalog_id query int false "filter by catalog id"
// @Param        teacher_id query int false "filter by teacher id"
// @Param        q query string false "search"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged offerings"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/offerings [get]
func (h *Handler) listOfferings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	list, total, err := h.store.PageOfferings(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list offerings", err)
		return
	}
	if list == nil {
		list = []OfferingView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

// exportOfferings streams all offerings as an xlsx download, using display names
// (course name, teaching class name) so the file round-trips with the importer.
func (h *Handler) exportOfferings(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListOfferings(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list offerings", err)
		return
	}
	headers := []string{"course", "teaching_class", "semester", "teacher", "course_seq", "teacher_id", "teacher_title", "college", "max_students", "requirement", "weekly_hours", "note"}
	rows := make([][]any, 0, len(list))
	for _, o := range list {
		rows = append(rows, []any{o.CatalogName, o.TeachingClassName, o.Semester, o.Teacher, o.CourseSeq, o.TeacherID, o.TeacherTitle, o.College, o.MaxStudents, o.Requirement, o.WeeklyHours, o.Note})
	}
	if err := xlsx.WriteExport(w, "offerings.xlsx", "offerings", headers, rows); err != nil {
		httpx.Error500(w, r, "could not export offerings", err)
	}
}

// @Summary      Create an offering
// @Tags         offerings
// @Accept       json
// @Produce      json
// @Param        body body OfferingInput true "offering input"
// @Success      201 {object} OfferingView "created offering"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / referenced catalog or teaching class not found"
// @Failure      409 {object} httpx.ErrorResponse "semester + catalog + teaching class already offered"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/offerings [post]
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
		httpx.Error500(w, r, "could not create offering", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建开课 %s", v.CatalogName))
	httpx.RespondJSON(w, http.StatusCreated, v)
}

// @Summary      Get an offering
// @Tags         offerings
// @Produce      json
// @Param        id path int true "offering id"
// @Success      200 {object} OfferingView "offering detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid offering id"
// @Failure      404 {object} httpx.ErrorResponse "offering not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/offerings/{id} [get]
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
		httpx.Error500(w, r, "could not load offering", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

// @Summary      Update an offering
// @Tags         offerings
// @Accept       json
// @Produce      json
// @Param        id path int true "offering id"
// @Param        body body OfferingInput true "offering input"
// @Success      200 {object} OfferingView "updated offering"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / referenced catalog or teaching class not found"
// @Failure      404 {object} httpx.ErrorResponse "offering not found"
// @Failure      409 {object} httpx.ErrorResponse "semester + catalog + teaching class already offered"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/offerings/{id} [put]
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
		httpx.Error500(w, r, "could not update offering", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新开课 %s", v.CatalogName))
	httpx.RespondJSON(w, http.StatusOK, v)
}

// @Summary      Delete an offering
// @Tags         offerings
// @Param        id path int true "offering id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid offering id"
// @Failure      404 {object} httpx.ErrorResponse "offering not found"
// @Failure      409 {object} httpx.ErrorResponse "sessions still exist for the offering"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/offerings/{id} [delete]
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
			httpx.Error500(w, r, "could not delete offering", err)
		}
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除开课 #%d", id))
	w.WriteHeader(http.StatusNoContent)
}

// ---- Sessions ----

// @Summary      List sessions
// @Tags         sessions
// @Produce      json
// @Param        offering_id query int false "filter by offering id"
// @Param        classroom_id query int false "filter by classroom id"
// @Param        from query string false "date range start (Y-M-D)"
// @Param        to query string false "date range end (Y-M-D)"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged sessions"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/sessions [get]
func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	f := sessionFilterFromQuery(q)
	f.Q = httpx.ParseSearch(q)
	list, total, err := h.store.PageSessions(r.Context(), f,
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list sessions", err)
		return
	}
	if list == nil {
		list = []SessionView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

// sessionFilterFromQuery parses the list/export filter params (offering_id,
// classroom_id, from, to), shared by listSessions and exportSessions so the two
// stay in sync. Search (q) and pagination are not read here — exports always
// cover the full filtered range.
func sessionFilterFromQuery(q url.Values) SessionFilter {
	offeringID, _ := strconv.ParseInt(q.Get("offering_id"), 10, 64)
	classroomID, _ := strconv.ParseInt(q.Get("classroom_id"), 10, 64)
	return SessionFilter{OfferingID: offeringID, ClassroomID: classroomID, From: q.Get("from"), To: q.Get("to")}
}

// querySessions parses the list/export filter params and returns the matching
// sessions, shared by listSessions and exportSessions so the two stay in sync.
func (h *Handler) querySessions(r *http.Request) ([]SessionView, error) {
	f := sessionFilterFromQuery(r.URL.Query())
	return h.store.ListSessions(r.Context(), f.OfferingID, f.ClassroomID, f.From, f.To)
}

// exportSessions streams sessions as an xlsx download, honoring the same
// filters (offering_id, classroom_id, from, to) as the list endpoint. Columns
// match the importer's headers (teacher included for human readability; the
// importer ignores unknown columns).
func (h *Handler) exportSessions(w http.ResponseWriter, r *http.Request) {
	list, err := h.querySessions(r)
	if err != nil {
		httpx.Error500(w, r, "could not list sessions", err)
		return
	}
	headers := []string{"date", "period_start", "period_end", "classroom", "course", "teaching_class", "semester", "teacher", "note"}
	rows := make([][]any, 0, len(list))
	for _, s := range list {
		rows = append(rows, []any{s.Date, s.PeriodStart, s.PeriodEnd, s.ClassroomName, s.CourseName, s.TeachingClassName, s.Semester, s.Teacher, s.Note})
	}
	if err := xlsx.WriteExport(w, "sessions.xlsx", "sessions", headers, rows); err != nil {
		httpx.Error500(w, r, "could not export sessions", err)
	}
}

// @Summary      Create a session
// @Tags         sessions
// @Accept       json
// @Produce      json
// @Param        body body SessionInput true "session input"
// @Success      201 {object} SessionView "created session"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / referenced offering or classroom not found"
// @Failure      409 {object} httpx.ErrorResponse "classroom occupied in the same periods"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/sessions [post]
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
		httpx.Error500(w, r, "could not create session", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建课次 %s %s", v.CourseName, v.Date))
	httpx.RespondJSON(w, http.StatusCreated, v)
}

// @Summary      Get a session
// @Tags         sessions
// @Produce      json
// @Param        id path int true "session id"
// @Success      200 {object} SessionView "session detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid session id"
// @Failure      404 {object} httpx.ErrorResponse "session not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/sessions/{id} [get]
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
		httpx.Error500(w, r, "could not load session", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

// @Summary      Update a session
// @Tags         sessions
// @Accept       json
// @Produce      json
// @Param        id path int true "session id"
// @Param        body body SessionInput true "session input"
// @Success      200 {object} SessionView "updated session"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / referenced offering or classroom not found"
// @Failure      404 {object} httpx.ErrorResponse "session not found"
// @Failure      409 {object} httpx.ErrorResponse "classroom occupied in the same periods"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/sessions/{id} [put]
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
		httpx.Error500(w, r, "could not update session", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新课次 %s %s", v.CourseName, v.Date))
	httpx.RespondJSON(w, http.StatusOK, v)
}

// @Summary      Delete a session
// @Tags         sessions
// @Param        id path int true "session id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid session id"
// @Failure      404 {object} httpx.ErrorResponse "session not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/sessions/{id} [delete]
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
		httpx.Error500(w, r, "could not delete session", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除课次 #%d", id))
	w.WriteHeader(http.StatusNoContent)
}

// ---- Timetable ----

// parseTimetableParams parses the classroom_id/from/to query params shared by
// the timetable JSON grid and its xlsx export, keeping their validation (and
// error messages) identical.
func parseTimetableParams(w http.ResponseWriter, r *http.Request) (classroomID int64, from, to string, ok bool) {
	q := r.URL.Query()
	classroomID, err := strconv.ParseInt(q.Get("classroom_id"), 10, 64)
	if err != nil || classroomID <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "classroom_id is required")
		return 0, "", "", false
	}
	from = q.Get("from")
	to = q.Get("to")
	if from == "" || to == "" {
		httpx.RespondError(w, http.StatusBadRequest, "from and to dates are required (YYYY-MM-DD)")
		return 0, "", "", false
	}
	return classroomID, from, to, true
}

// @Summary      Classroom timetable for a date range
// @Tags         sessions
// @Produce      json
// @Param        classroom_id query int true "classroom id"
// @Param        from query string true "range start (Y-M-D)"
// @Param        to query string true "range end (Y-M-D)"
// @Success      200 {array} TimetableDay "one entry per day in range"
// @Failure      400 {object} httpx.ErrorResponse "classroom_id required / invalid date range"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/timetable [get]
func (h *Handler) timetable(w http.ResponseWriter, r *http.Request) {
	classroomID, from, to, ok := parseTimetableParams(w, r)
	if !ok {
		return
	}
	days, err := h.store.Timetable(r.Context(), classroomID, from, to, h.regimes)
	if err != nil {
		httpx.Error500(w, r, "could not build timetable", err)
		return
	}
	if days == nil {
		days = []TimetableDay{}
	}
	httpx.RespondJSON(w, http.StatusOK, days)
}

// timetableExport streams the weekly grid as an xlsx download replicating the
// browser table: days as columns, periods as rows, multi-period sessions
// merged into one cell. The filename carries the classroom name and the date
// range shown on the page.
func (h *Handler) timetableExport(w http.ResponseWriter, r *http.Request) {
	classroomID, from, to, ok := parseTimetableParams(w, r)
	if !ok {
		return
	}
	cr, err := h.classrooms.GetByID(r.Context(), classroomID)
	if errors.Is(err, classroom.ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load classroom", err)
		return
	}
	days, err := h.store.Timetable(r.Context(), classroomID, from, to, h.regimes)
	if err != nil {
		httpx.Error500(w, r, "could not build timetable", err)
		return
	}
	display := timetableExportFilename(cr.Name, from, to)
	if err := xlsx.WriteCustom(w, "classroom-timetable.xlsx", display, populateTimetable(days)); err != nil {
		httpx.Error500(w, r, "could not export timetable", err)
	}
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
