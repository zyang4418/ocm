package classroom

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
	"ocm-backend/internal/xlsx"
)

var validTypes = map[string]bool{
	TypeStandard:    true,
	TypeMultimedia:  true,
	TypeComputer:    true,
	TypeLab:         true,
	TypeLectureHall: true,
	TypeStadium:     true,
	TypeDrawing:     true,
	TypeLanguage:    true,
	TypeStudio:      true,
	TypeSpecial:     true,
}

var validStatuses = map[string]bool{
	StatusAvailable:   true,
	StatusMaintenance: true,
	StatusDisabled:    true,
}

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts the classroom endpoints on mux. Reading (list/get)
// requires classroom:read, which every authenticated role has; management
// (create/update/delete) requires classroom:manage, granted only to admins.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.ClassroomRead)(http.HandlerFunc(handler)))
	}
	manage := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.ClassroomManage)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/classrooms", read(h.list))
	mux.Handle("POST /api/classrooms", manage(h.create))
	mux.Handle("GET /api/classrooms/export", read(h.export))
	mux.Handle("GET /api/classrooms/{id}", read(h.get))
	mux.Handle("PUT /api/classrooms/{id}", manage(h.update))
	mux.Handle("DELETE /api/classrooms/{id}", manage(h.delete))
}

// @Summary      List classrooms
// @Tags         classrooms
// @Produce      json
// @Param        q query string false "search by name/building/campus"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged classrooms"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/classrooms [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	classrooms, total, err := h.store.PageClassrooms(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list classrooms", err)
		return
	}
	if classrooms == nil {
		classrooms = []Classroom{}
	}
	httpx.RespondPaged(w, classrooms, total, p)
}

// export streams all classrooms as an xlsx download. The column layout matches
// the importer's expected headers so the file round-trips.
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	classrooms, err := h.store.List(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list classrooms", err)
		return
	}
	headers := []string{"name", "building", "capacity", "type", "floor", "campus", "status", "description"}
	rows := make([][]any, 0, len(classrooms))
	for _, c := range classrooms {
		rows = append(rows, []any{c.Name, c.Building, c.Capacity, c.Type, c.Floor, c.Campus, c.Status, c.Description})
	}
	if err := xlsx.WriteExport(w, "classrooms.xlsx", "classrooms", headers, rows); err != nil {
		httpx.Error500(w, r, "could not export classrooms", err)
	}
}

// @Summary      Create a classroom
// @Tags         classrooms
// @Accept       json
// @Produce      json
// @Param        body body ClassroomInput true "classroom input"
// @Success      201 {object} Classroom "created classroom"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / name required / capacity must be positive"
// @Failure      409 {object} httpx.ErrorResponse "classroom name already taken"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/classrooms [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in ClassroomInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeInput(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := h.store.Create(r.Context(), in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "classroom name already taken")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not create classroom", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建教室 %s", c.Name))
	httpx.RespondJSON(w, http.StatusCreated, c)
}

// @Summary      Get a classroom
// @Tags         classrooms
// @Produce      json
// @Param        id path int true "classroom id"
// @Success      200 {object} Classroom "classroom detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid classroom id"
// @Failure      404 {object} httpx.ErrorResponse "classroom not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/classrooms/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid classroom id")
		return
	}
	c, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load classroom", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

// @Summary      Update a classroom
// @Tags         classrooms
// @Accept       json
// @Produce      json
// @Param        id path int true "classroom id"
// @Param        body body ClassroomInput true "classroom input"
// @Success      200 {object} Classroom "updated classroom"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / name required"
// @Failure      404 {object} httpx.ErrorResponse "classroom not found"
// @Failure      409 {object} httpx.ErrorResponse "classroom name already taken"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/classrooms/{id} [put]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid classroom id")
		return
	}
	var in ClassroomInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeInput(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := h.store.Update(r.Context(), id, in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "classroom name already taken")
		return
	}
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update classroom", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新教室 %s", c.Name))
	httpx.RespondJSON(w, http.StatusOK, c)
}

// @Summary      Delete a classroom
// @Tags         classrooms
// @Param        id path int true "classroom id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid classroom id"
// @Failure      404 {object} httpx.ErrorResponse "classroom not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/classrooms/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid classroom id")
		return
	}
	existing, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load classroom", err)
		return
	}
	err = h.store.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not delete classroom", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除教室 %s", existing.Name))
	w.WriteHeader(http.StatusNoContent)
}

// NormalizeInput trims string fields, applies defaults for type and status,
// and validates the result. It returns an error message when invalid. Exported
// so the import framework reuses the exact same rules as the CRUD handlers.
func NormalizeInput(in *ClassroomInput) (string, bool) {
	return normalizeInput(in)
}

// normalizeInput trims string fields, applies defaults for type and status,
// and validates the result. It returns an error message when invalid.
func normalizeInput(in *ClassroomInput) (string, bool) {
	in.Name = strings.TrimSpace(in.Name)
	in.Building = strings.TrimSpace(in.Building)
	in.Floor = strings.TrimSpace(in.Floor)
	in.Campus = strings.TrimSpace(in.Campus)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" {
		return "name is required", false
	}
	if in.Capacity <= 0 {
		return "capacity must be greater than 0", false
	}
	if in.Type == "" {
		in.Type = TypeStandard
	} else if !validTypes[in.Type] {
		return "invalid classroom type", false
	}
	if in.Status == "" {
		in.Status = StatusAvailable
	} else if !validStatuses[in.Status] {
		return "invalid classroom status", false
	}
	return "", true
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
