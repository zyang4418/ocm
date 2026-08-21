package classroom

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
)

// RepairHandler serves the repair ticket endpoints. Submitting requires
// repair:create; assigning/processing requires repair:assign; reading requires
// either (repair:create is scoped to one's own tickets, repair:assign sees
// all).
type RepairHandler struct {
	store *RepairStore
}

func NewRepairHandler(store *RepairStore) *RepairHandler { return &RepairHandler{store: store} }

// RegisterRoutes mounts the repair endpoints on mux.
func (h *RepairHandler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	create := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.RepairCreate)(http.HandlerFunc(handler)))
	}
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAny(authz.RepairCreate, authz.RepairAssign)(http.HandlerFunc(handler)))
	}
	manage := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.RepairAssign)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/repairs", read(h.list))
	mux.Handle("POST /api/repairs", create(h.create))
	mux.Handle("POST /api/repairs/emergency", create(h.emergency))
	mux.Handle("GET /api/repairs/{id}", read(h.get))
	mux.Handle("PUT /api/repairs/{id}", manage(h.update))
	mux.Handle("POST /api/repairs/{id}/confirm", create(h.confirm))
}

// isRepairAdmin reports whether the subject holds repair:assign (sees and
// processes every ticket).
func isRepairAdmin(r *http.Request) bool {
	s, ok := authz.SubjectFrom(r.Context())
	return ok && s.Has(authz.RepairAssign)
}

// @Summary      List repair tickets
// @Tags         repairs
// @Produce      json
// @Param        classroom_id query int false "filter by classroom id"
// @Param        status query string false "filter by status" Enums(open,processing,completed,confirmed)
// @Param        q query string false "search"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged repair views"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/repairs [get]
func (h *RepairHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := RepairFilter{
		ClassroomID: queryInt(q, "classroom_id"),
		Status:      strings.TrimSpace(q.Get("status")),
	}
	p := httpx.ParsePageParams(q)
	subject, _ := authz.SubjectFrom(r.Context())
	list, total, err := h.store.Page(r.Context(), f, httpx.ParseSearch(q), subject.ID, isRepairAdmin(r),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list repairs", err)
		return
	}
	if list == nil {
		list = []RepairView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

// @Summary      Get a repair ticket
// @Tags         repairs
// @Produce      json
// @Param        id path int true "repair id"
// @Success      200 {object} RepairView "repair detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid repair id"
// @Failure      404 {object} httpx.ErrorResponse "repair ticket not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/repairs/{id} [get]
func (h *RepairHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseRepairID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid repair id")
		return
	}
	v, err := h.store.Get(r.Context(), id)
	if errors.Is(err, ErrRepairNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "repair ticket not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load repair", err)
		return
	}
	if !isRepairAdmin(r) && !isRepairCreator(r, v) {
		// Non-admins can only see their own tickets; hide others as 404.
		httpx.RespondError(w, http.StatusNotFound, "repair ticket not found")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

// @Summary      Create a repair ticket
// @Tags         repairs
// @Accept       json
// @Produce      json
// @Param        body body RepairInput true "repair input"
// @Success      201 {object} RepairView "created repair view"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / classroom not found"
// @Failure      409 {object} httpx.ErrorResponse "open ticket already exists for the classroom"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/repairs [post]
func (h *RepairHandler) create(w http.ResponseWriter, r *http.Request) {
	var in RepairInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeRepairInput(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.Create(r.Context(), in, subject.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "classroom not found")
	case errors.Is(err, ErrRepairOpenExists):
		httpx.RespondError(w, http.StatusConflict, "该教室已有待处理报修，请勿重复提交。")
	case err != nil:
		httpx.Error500(w, r, "could not create repair", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("提交教室报修 #%d", v.ID))
		httpx.RespondJSON(w, http.StatusCreated, v)
	}
}

// @Summary      Create an emergency repair ticket (skips the open-ticket guard)
// @Tags         repairs
// @Accept       json
// @Produce      json
// @Param        body body RepairInput true "repair input"
// @Success      201 {object} RepairView "created repair view"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / classroom not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/repairs/emergency [post]
func (h *RepairHandler) emergency(w http.ResponseWriter, r *http.Request) {
	var in RepairInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizeRepairInput(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.EmergencyCreate(r.Context(), in, subject.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(w, http.StatusBadRequest, "classroom not found")
	case err != nil:
		httpx.Error500(w, r, "could not create emergency repair", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("提交紧急报修 #%d", v.ID))
		httpx.RespondJSON(w, http.StatusCreated, v)
	}
}

// @Summary      Process a repair ticket (start/finish)
// @Tags         repairs
// @Accept       json
// @Produce      json
// @Param        id path int true "repair id"
// @Param        body body RepairUpdateInput true "status transition"
// @Success      200 {object} RepairView "updated repair view"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / invalid status transition"
// @Failure      404 {object} httpx.ErrorResponse "repair ticket not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/repairs/{id} [put]
func (h *RepairHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseRepairID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid repair id")
		return
	}
	var in RepairUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Status = strings.TrimSpace(in.Status)
	in.Remark = strings.TrimSpace(in.Remark)
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.Update(r.Context(), id, in, subject.ID)
	switch {
	case errors.Is(err, ErrRepairNotFound):
		httpx.RespondError(w, http.StatusNotFound, "repair ticket not found")
	case errors.Is(err, ErrRepairState):
		httpx.RespondError(w, http.StatusBadRequest, "invalid repair status transition")
	case err != nil:
		httpx.Error500(w, r, "could not update repair", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("处理教室报修 #%d", id))
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

// @Summary      Confirm a completed repair (creator only)
// @Tags         repairs
// @Produce      json
// @Param        id path int true "repair id"
// @Success      200 {object} RepairView "confirmed repair view"
// @Failure      400 {object} httpx.ErrorResponse "repair not completed yet"
// @Failure      403 {object} httpx.ErrorResponse "only the creator may confirm"
// @Failure      404 {object} httpx.ErrorResponse "repair ticket not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/repairs/{id}/confirm [post]
func (h *RepairHandler) confirm(w http.ResponseWriter, r *http.Request) {
	id, err := parseRepairID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid repair id")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.Confirm(r.Context(), id, subject.ID)
	switch {
	case errors.Is(err, ErrRepairNotFound):
		httpx.RespondError(w, http.StatusNotFound, "repair ticket not found")
	case errors.Is(err, ErrRepairForbidden):
		httpx.RespondError(w, http.StatusForbidden, "只有报修人才能确认完成")
	case errors.Is(err, ErrRepairState):
		httpx.RespondError(w, http.StatusBadRequest, "只有已完成的报修才能确认")
	case err != nil:
		httpx.Error500(w, r, "could not confirm repair", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("确认报修完成 #%d", id))
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

func isRepairCreator(r *http.Request, v RepairView) bool {
	s, ok := authz.SubjectFrom(r.Context())
	return ok && s.ID == v.CreatorID
}

func parseRepairID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func queryInt(q url.Values, key string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(q.Get(key)), 10, 64)
	return n
}

// normalizeRepairInput trims and validates the create/emergency payload.
func normalizeRepairInput(in *RepairInput) (string, bool) {
	in.Description = strings.TrimSpace(in.Description)
	if in.ClassroomID <= 0 {
		return "classroomId is required", false
	}
	if in.Description == "" {
		return "description is required", false
	}
	return "", true
}
