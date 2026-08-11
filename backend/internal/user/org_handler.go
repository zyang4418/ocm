package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/xlsx"
)

// registerOrgRoutes mounts the admin-class and teaching-class endpoints. They
// live in the user/people module and use their own permission constants,
// independent of course management. Reading requires the corresponding read
// permission (granted to every authenticated role so teachers can browse class
// names); management requires the manage permission (admin only).
func (h *Handler) registerOrgRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(perm string, handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(perm)(http.HandlerFunc(handler)))
	}
	// Admin classes (行政班)
	mux.Handle("GET /api/admin-classes", read(authz.AdminClassRead, h.listAdminClasses))
	mux.Handle("POST /api/admin-classes", read(authz.AdminClassManage, h.createAdminClass))
	mux.Handle("GET /api/admin-classes/export", read(authz.AdminClassRead, h.exportAdminClasses))
	mux.Handle("GET /api/admin-classes/{id}", read(authz.AdminClassRead, h.getAdminClass))
	mux.Handle("PUT /api/admin-classes/{id}", read(authz.AdminClassManage, h.updateAdminClass))
	mux.Handle("DELETE /api/admin-classes/{id}", read(authz.AdminClassManage, h.deleteAdminClass))
	// Teaching classes (教学班)
	mux.Handle("GET /api/teaching-classes", read(authz.TeachingClassRead, h.listTeachingClasses))
	mux.Handle("POST /api/teaching-classes", read(authz.TeachingClassManage, h.createTeachingClass))
	mux.Handle("GET /api/teaching-classes/export", read(authz.TeachingClassRead, h.exportTeachingClasses))
	mux.Handle("GET /api/teaching-classes/{id}", read(authz.TeachingClassRead, h.getTeachingClass))
	mux.Handle("PUT /api/teaching-classes/{id}", read(authz.TeachingClassManage, h.updateTeachingClass))
	mux.Handle("DELETE /api/teaching-classes/{id}", read(authz.TeachingClassManage, h.deleteTeachingClass))
}

// ---- Admin classes ----

func (h *Handler) listAdminClasses(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListAdminClasses(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list admin classes")
		return
	}
	if list == nil {
		list = []AdminClass{}
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

// exportAdminClasses streams all admin classes as an xlsx download. Columns
// match the importer's headers so the file round-trips.
func (h *Handler) exportAdminClasses(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListAdminClasses(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list admin classes")
		return
	}
	headers := []string{"grade", "name", "note"}
	rows := make([][]any, 0, len(list))
	for _, c := range list {
		rows = append(rows, []any{c.Grade, c.Name, c.Note})
	}
	if err := xlsx.WriteExport(w, "admin-classes.xlsx", "admin-classes", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export admin classes")
	}
}

func (h *Handler) createAdminClass(w http.ResponseWriter, r *http.Request) {
	var in AdminClassInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeAdminClass(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := h.store.CreateAdminClass(r.Context(), in)
	if errors.Is(err, ErrClassNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "该年级下行政班名已存在")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create admin class")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, c)
}

func (h *Handler) getAdminClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	c, err := h.store.GetAdminClass(r.Context(), id)
	if errors.Is(err, ErrAdminClassNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "admin class not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load admin class")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

func (h *Handler) updateAdminClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	var in AdminClassInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeAdminClass(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := h.store.UpdateAdminClass(r.Context(), id, in)
	if errors.Is(err, ErrClassNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "该年级下行政班名已存在")
		return
	}
	if errors.Is(err, ErrAdminClassNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "admin class not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update admin class")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, c)
}

func (h *Handler) deleteAdminClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	if err := h.store.DeleteAdminClass(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, ErrClassInUse):
			httpx.RespondError(w, http.StatusConflict, "该行政班已被教学班引用，无法删除")
		case errors.Is(err, ErrAdminClassNotFound):
			httpx.RespondError(w, http.StatusNotFound, "admin class not found")
		default:
			httpx.RespondError(w, http.StatusInternalServerError, "could not delete admin class")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeAdminClass(in *AdminClassInput) (string, bool) {
	in.Grade = strings.TrimSpace(in.Grade)
	in.Name = strings.TrimSpace(in.Name)
	in.Note = strings.TrimSpace(in.Note)
	if in.Name == "" {
		return "name is required", false
	}
	return "", true
}

// NormalizeAdminClass is the exported wrapper around normalizeAdminClass so the
// import framework reuses the same validation rules as the CRUD handlers.
func NormalizeAdminClass(in *AdminClassInput) (string, bool) {
	return normalizeAdminClass(in)
}

// ---- Teaching classes ----

func (h *Handler) listTeachingClasses(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListTeachingClasses(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list teaching classes")
		return
	}
	if list == nil {
		list = []TeachingClassView{}
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

// exportTeachingClasses streams all teaching classes as an xlsx download,
// flattened to one row per member admin class (parent columns repeated). A
// teaching class with N members produces N rows; the layout round-trips with
// the importer, which groups by name and replaces the member set.
func (h *Handler) exportTeachingClasses(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListTeachingClasses(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list teaching classes")
		return
	}
	headers := []string{"name", "note", "admin_grade", "admin_name"}
	rows := make([][]any, 0, len(list))
	for _, tc := range list {
		for _, m := range tc.Classes {
			rows = append(rows, []any{tc.Name, tc.Note, m.Grade, m.Name})
		}
	}
	if err := xlsx.WriteExport(w, "teaching-classes.xlsx", "teaching-classes", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export teaching classes")
	}
}

func (h *Handler) createTeachingClass(w http.ResponseWriter, r *http.Request) {
	var in TeachingClassInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeTeachingClass(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	v, err := h.store.CreateTeachingClass(r.Context(), in)
	if errors.Is(err, ErrClassNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "教学班名已存在")
		return
	}
	if errors.Is(err, ErrMemberRequired) {
		httpx.RespondError(w, http.StatusBadRequest, "至少选择一个行政班")
		return
	}
	if errors.Is(err, ErrAdminClassNotFound) {
		httpx.RespondError(w, http.StatusBadRequest, "包含不存在的行政班")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create teaching class")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, v)
}

func (h *Handler) getTeachingClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid teaching class id")
		return
	}
	v, err := h.store.GetTeachingClass(r.Context(), id)
	if errors.Is(err, ErrTeachingClassNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "teaching class not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load teaching class")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) updateTeachingClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid teaching class id")
		return
	}
	var in TeachingClassInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeTeachingClass(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	v, err := h.store.UpdateTeachingClass(r.Context(), id, in)
	if errors.Is(err, ErrClassNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "教学班名已存在")
		return
	}
	if errors.Is(err, ErrClassInUse) {
		httpx.RespondError(w, http.StatusConflict, "教学班已被开课引用，成员不可修改，请新建教学班")
		return
	}
	if errors.Is(err, ErrTeachingClassNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "teaching class not found")
		return
	}
	if errors.Is(err, ErrMemberRequired) {
		httpx.RespondError(w, http.StatusBadRequest, "至少选择一个行政班")
		return
	}
	if errors.Is(err, ErrAdminClassNotFound) {
		httpx.RespondError(w, http.StatusBadRequest, "包含不存在的行政班")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update teaching class")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) deleteTeachingClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid teaching class id")
		return
	}
	if err := h.store.DeleteTeachingClass(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, ErrClassInUse):
			httpx.RespondError(w, http.StatusConflict, "教学班已被开课引用，无法删除")
		case errors.Is(err, ErrTeachingClassNotFound):
			httpx.RespondError(w, http.StatusNotFound, "teaching class not found")
		default:
			httpx.RespondError(w, http.StatusInternalServerError, "could not delete teaching class")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeTeachingClass(in *TeachingClassInput) (string, bool) {
	in.Name = strings.TrimSpace(in.Name)
	in.Note = strings.TrimSpace(in.Note)
	in.ClassIDs = dedupInt64(in.ClassIDs)
	if in.Name == "" {
		return "name is required", false
	}
	if len(in.ClassIDs) == 0 {
		return "至少选择一个行政班", false
	}
	return "", true
}

// NormalizeTeachingClass is the exported wrapper around normalizeTeachingClass
// so the import framework reuses the same validation rules. Importers that need
// to relax the member-required check (e.g. to warn instead of fail) do so after
// calling this.
func NormalizeTeachingClass(in *TeachingClassInput) (string, bool) {
	return normalizeTeachingClass(in)
}

func dedupInt64(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
