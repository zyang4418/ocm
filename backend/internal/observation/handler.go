package observation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
)

// Handler serves the observation endpoints. Writing (create/update/delete/
// submit) requires observation:write and is scoped to the actor's own records;
// reading requires observation:read (own) or observation:manage (all records +
// exporting anyone's).
type Handler struct {
	store    *Store
	renderer Renderer
}

func NewHandler(store *Store, renderer Renderer) *Handler {
	return &Handler{store: store, renderer: renderer}
}

// RegisterRoutes mounts the observation endpoints.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAny(authz.ObservationRead, authz.ObservationManage)(http.HandlerFunc(handler)))
	}
	write := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.ObservationWrite)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/observations/templates", read(h.templates))
	mux.Handle("GET /api/observations", read(h.list))
	mux.Handle("POST /api/observations", write(h.create))
	mux.Handle("GET /api/observations/{id}", read(h.get))
	mux.Handle("PUT /api/observations/{id}", write(h.update))
	mux.Handle("DELETE /api/observations/{id}", write(h.delete))
	mux.Handle("POST /api/observations/{id}/submit", write(h.submit))
	mux.Handle("POST /api/observations/{id}/export", read(h.export))
}

// isAdmin reports whether the request's subject holds observation:manage.
func isAdmin(r *http.Request) bool {
	s, ok := authz.SubjectFrom(r.Context())
	return ok && s.Has(authz.ObservationManage)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (h *Handler) templates(w http.ResponseWriter, r *http.Request) {
	if h.renderer == nil {
		httpx.RespondError(w, http.StatusNotImplemented, "observation document backend not configured")
		return
	}
	t, err := h.renderer.Templates()
	if err != nil {
		httpx.Error500(w, r, "could not load observation templates", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, t)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{
		Status:       strings.TrimSpace(q.Get("status")),
		TemplateType: strings.TrimSpace(q.Get("template_type")),
		CourseID:     queryInt(q, "course_id"),
		From:         strings.TrimSpace(q.Get("start_date")),
		To:           strings.TrimSpace(q.Get("end_date")),
	}
	if f.From != "" {
		if _, err := time.Parse("2006-01-02", f.From); err != nil {
			httpx.RespondError(w, http.StatusBadRequest, "invalid start_date (use YYYY-MM-DD)")
			return
		}
	}
	if f.To != "" {
		if _, err := time.Parse("2006-01-02", f.To); err != nil {
			httpx.RespondError(w, http.StatusBadRequest, "invalid end_date (use YYYY-MM-DD)")
			return
		}
	}
	p := httpx.ParsePageParams(q)
	subject, _ := authz.SubjectFrom(r.Context())
	list, total, err := h.store.Page(r.Context(), f, httpx.ParseSearch(q), subject.ID, isAdmin(r),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list observations", err)
		return
	}
	if list == nil {
		list = []ObservationView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	v, err := h.store.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load observation", err)
		return
	}
	if !isAdmin(r) && !isOwn(r, v) {
		// A non-admin can only see their own records; hide others as 404.
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in ObservationInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.Create(r.Context(), in, subject.ID, subject.DisplayName)
	switch {
	case errors.Is(err, ErrCourseNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "course offering not found")
	case errors.Is(err, ErrSessionNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "session not found")
	case errors.Is(err, ErrClassroomNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "classroom not found")
	case errors.Is(err, ErrSessionMismatch):
		httpx.RespondError(w, http.StatusBadRequest, "session does not belong to the course offering")
	case errors.Is(err, ErrClassroomMismatch):
		httpx.RespondError(w, http.StatusBadRequest, "classroom does not match the session")
	case errors.Is(err, ErrDuplicate):
		httpx.RespondError(w, http.StatusConflict, "该节课已存在评课记录，不能重复评课。")
	case err != nil:
		httpx.Error500(w, r, "could not create observation", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("创建评课 #%d（%s）", v.ID, v.TemplateType))
		httpx.RespondJSON(w, http.StatusCreated, v)
	}
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	var in ObservationInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.Update(r.Context(), id, in, subject.ID, subject.DisplayName)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
	case errors.Is(err, ErrForbidden):
		httpx.RespondError(w, http.StatusForbidden, "only the observer can edit this observation")
	case errors.Is(err, ErrSubmitted):
		httpx.RespondError(w, http.StatusConflict, "submitted observation cannot be edited")
	case errors.Is(err, ErrCourseNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "course offering not found")
	case errors.Is(err, ErrSessionNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "session not found")
	case errors.Is(err, ErrClassroomNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "classroom not found")
	case errors.Is(err, ErrSessionMismatch):
		httpx.RespondError(w, http.StatusBadRequest, "session does not belong to the course offering")
	case errors.Is(err, ErrClassroomMismatch):
		httpx.RespondError(w, http.StatusBadRequest, "classroom does not match the session")
	case errors.Is(err, ErrDuplicate):
		httpx.RespondError(w, http.StatusConflict, "该节课已存在评课记录，不能重复评课。")
	case err != nil:
		httpx.Error500(w, r, "could not update observation", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("编辑评课 #%d", id))
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	err = h.store.Delete(r.Context(), id, subject.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
	case errors.Is(err, ErrForbidden):
		httpx.RespondError(w, http.StatusForbidden, "only the observer can delete this observation")
	case errors.Is(err, ErrSubmitted):
		httpx.RespondError(w, http.StatusConflict, "submitted observation cannot be deleted")
	case err != nil:
		httpx.Error500(w, r, "could not delete observation", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("删除评课 #%d", id))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())

	// Load once for validation + ownership; the store re-checks on submit.
	current, err := h.store.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load observation", err)
		return
	}
	if current.ObserverID != subject.ID {
		httpx.RespondError(w, http.StatusForbidden, "only the observer can submit this observation")
		return
	}
	if h.renderer != nil {
		obs := current.Observation
		if missing, err := h.renderer.Validate(&obs); err != nil {
			httpx.Error500(w, r, "could not validate observation", err)
			return
		} else if len(missing) > 0 {
			httpx.RespondJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "missing required fields",
				"missing": missing,
			})
			return
		}
	}

	v, err := h.store.Submit(r.Context(), id, subject.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
	case errors.Is(err, ErrForbidden):
		httpx.RespondError(w, http.StatusForbidden, "only the observer can submit this observation")
	case err != nil:
		httpx.Error500(w, r, "could not submit observation", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("提交评课 #%d", id))
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	v, err := h.store.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load observation", err)
		return
	}
	if !isAdmin(r) && !isOwn(r, v) {
		httpx.RespondError(w, http.StatusNotFound, "observation not found")
		return
	}
	if v.Status != StatusSubmitted {
		httpx.RespondError(w, http.StatusBadRequest, "only submitted observation can be exported")
		return
	}
	if h.renderer == nil {
		httpx.RespondError(w, http.StatusNotImplemented, "observation document backend not configured")
		return
	}

	var buf bytes.Buffer
	if err := h.renderer.Render(r.Context(), &v.Observation, &buf); err != nil {
		httpx.Error500(w, r, "could not render observation document", err)
		return
	}
	if err := h.store.MarkExported(r.Context(), id); err != nil {
		httpx.Error500(w, r, "could not record export", err)
		return
	}
	serveDocx(w, fmt.Sprintf("observation-%d-%s.docx", id, v.TemplateType), buf.Bytes())
}

// isOwn reports whether the request's subject is the observation's observer.
func isOwn(r *http.Request, v ObservationView) bool {
	s, ok := authz.SubjectFrom(r.Context())
	return ok && s.ID == v.ObserverID
}

func queryInt(q url.Values, key string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(q.Get(key)), 10, 64)
	return n
}
