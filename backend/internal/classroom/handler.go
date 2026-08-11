package classroom

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/xlsx"
)

var validTypes = map[string]bool{
	TypeStandard:    true,
	TypeMultimedia:  true,
	TypeComputer:    true,
	TypeLab:         true,
	TypeLectureHall: true,
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

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	classrooms, err := h.store.List(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list classrooms")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, classrooms)
}

// export streams all classrooms as an xlsx download. The column layout matches
// the importer's expected headers so the file round-trips.
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	classrooms, err := h.store.List(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list classrooms")
		return
	}
	headers := []string{"name", "building", "capacity", "type", "status", "description"}
	rows := make([][]any, 0, len(classrooms))
	for _, c := range classrooms {
		rows = append(rows, []any{c.Name, c.Building, c.Capacity, c.Type, c.Status, c.Description})
	}
	if err := xlsx.WriteExport(w, "classrooms.xlsx", "classrooms", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export classrooms")
	}
}

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
		httpx.RespondError(w, http.StatusInternalServerError, "could not create classroom")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, c)
}

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
		httpx.RespondError(w, http.StatusInternalServerError, "could not load classroom")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

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
		httpx.RespondError(w, http.StatusInternalServerError, "could not update classroom")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid classroom id")
		return
	}
	err = h.store.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not delete classroom")
		return
	}
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
