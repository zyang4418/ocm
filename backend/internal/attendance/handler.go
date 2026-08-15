package attendance

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

// Handler serves the attendance endpoints. Reading requires attendance:read;
// managing (create/close/correct records) requires attendance:manage; a
// student scans with attendance:checkin (teachers may also scan via manage).
type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts the checkin endpoints. Literal segments (/me,
// /summary, /export, /scan) take precedence over the {id} wildcard, so they
// coexist on the same path prefix (same as GET /api/courses/export).
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.AttendanceRead)(http.HandlerFunc(handler)))
	}
	manage := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.AttendanceManage)(http.HandlerFunc(handler)))
	}
	scan := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAny(authz.AttendanceCheckin, authz.AttendanceManage)(http.HandlerFunc(handler)))
	}
	my := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequireAny(authz.AttendanceCheckin, authz.AttendanceRead)(http.HandlerFunc(handler)))
	}

	mux.Handle("POST /api/checkins", manage(h.createCheckin))
	mux.Handle("GET /api/checkins", read(h.listCheckins))
	mux.Handle("GET /api/checkins/me", my(h.myCheckins))
	mux.Handle("GET /api/checkins/summary", read(h.offeringSummary))
	mux.Handle("GET /api/checkins/export", read(h.exportOfferingReport))
	mux.Handle("POST /api/checkins/scan", scan(h.scan))
	mux.Handle("GET /api/checkins/{id}", read(h.getCheckin))
	mux.Handle("POST /api/checkins/{id}/close", manage(h.closeCheckin))
	mux.Handle("GET /api/checkins/{id}/records", read(h.listRecords))
	mux.Handle("PUT /api/checkins/{id}/records/{userId}", manage(h.updateRecord))
	mux.Handle("GET /api/checkins/{id}/export", read(h.exportCheckin))
}

// ---- Checkins ----

func (h *Handler) createCheckin(w http.ResponseWriter, r *http.Request) {
	var in CheckinInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.CreateCheckin(r.Context(), in, subject.ID)
	switch {
	case errors.Is(err, ErrTitleRequired):
		httpx.RespondError(w, http.StatusBadRequest, "独立签到需填写标题")
		return
	case errors.Is(err, ErrOfferingMismatch):
		httpx.RespondError(w, http.StatusBadRequest, "课次不属于所选开课")
		return
	case errors.Is(err, ErrCheckinNotFound):
		httpx.RespondError(w, http.StatusNotFound, "开课或课次不存在")
		return
	case err != nil:
		httpx.Error500(w, r, "could not create checkin", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("发起签到 %s", v.Title))
	httpx.RespondJSON(w, http.StatusCreated, v)
}

func (h *Handler) listCheckins(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	f := CheckinFilter{
		OfferingID: queryInt(q, "offering_id"),
		SessionID:  queryInt(q, "session_id"),
		Status:     strings.TrimSpace(q.Get("status")),
		From:       strings.TrimSpace(q.Get("from")),
		To:         strings.TrimSpace(q.Get("to")),
		Q:          httpx.ParseSearch(q),
	}
	if f.Status != "" && f.Status != StatusActive && f.Status != StatusClosed {
		httpx.RespondError(w, http.StatusBadRequest, "invalid status filter")
		return
	}
	list, total, err := h.store.PageCheckins(r.Context(), f,
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list checkins", err)
		return
	}
	if list == nil {
		list = []CheckinView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

func (h *Handler) getCheckin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid checkin id")
		return
	}
	v, err := h.store.GetCheckin(r.Context(), id)
	if errors.Is(err, ErrCheckinNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "checkin not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load checkin", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) closeCheckin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid checkin id")
		return
	}
	err = h.store.CloseCheckin(r.Context(), id)
	switch {
	case errors.Is(err, ErrCheckinNotFound):
		httpx.RespondError(w, http.StatusNotFound, "checkin not found")
	case errors.Is(err, ErrCheckinNotActive):
		httpx.RespondError(w, http.StatusConflict, "签到已结束")
	case err != nil:
		httpx.Error500(w, r, "could not close checkin", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("结束签到 #%d", id))
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- Scan ----

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	code := strings.TrimSpace(in.Code)
	if len(code) != 6 {
		httpx.RespondError(w, http.StatusBadRequest, "签到码为 6 位数字")
		return
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			httpx.RespondError(w, http.StatusBadRequest, "签到码为 6 位数字")
			return
		}
	}
	subject, _ := authz.SubjectFrom(r.Context())
	res, err := h.store.ScanByCode(r.Context(), code, subject.ID)
	switch {
	case errors.Is(err, ErrCodeNotFound):
		httpx.RespondError(w, http.StatusNotFound, "签到码无效")
	case errors.Is(err, ErrCheckinNotActive):
		httpx.RespondError(w, http.StatusConflict, "签到已结束")
	case errors.Is(err, ErrCheckinExpired):
		httpx.RespondError(w, http.StatusConflict, "签到已过期")
	case err != nil:
		httpx.Error500(w, r, "could not scan checkin", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("扫码签到 %s", res.Title))
		httpx.RespondJSON(w, http.StatusOK, res)
	}
}

// ---- Records ----

func (h *Handler) listRecords(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid checkin id")
		return
	}
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	f := RecordFilter{Status: strings.TrimSpace(q.Get("status")), Q: httpx.ParseSearch(q)}
	if f.Status != "" && !validRecordStatus(f.Status) {
		httpx.RespondError(w, http.StatusBadRequest, "invalid status filter")
		return
	}
	list, total, err := h.store.PageRecords(r.Context(), id, f,
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if errors.Is(err, ErrCheckinNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "checkin not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not list checkin records", err)
		return
	}
	if list == nil {
		list = []CheckinRecordView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

func (h *Handler) updateRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid checkin id")
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validRecordStatus(in.Status) {
		httpx.RespondError(w, http.StatusBadRequest, "invalid record status")
		return
	}
	subject, _ := authz.SubjectFrom(r.Context())
	v, err := h.store.UpsertRecord(r.Context(), id, userID, in.Status, subject.ID)
	switch {
	case errors.Is(err, ErrCheckinNotFound):
		httpx.RespondError(w, http.StatusNotFound, "checkin not found")
	case errors.Is(err, ErrStudentNotFound):
		httpx.RespondError(w, http.StatusNotFound, "用户不存在")
	case err != nil:
		httpx.Error500(w, r, "could not update checkin record", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("修正签到记录 #%d 学生 #%d → %s", id, userID, statusLabel(v.Status)))
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

// ---- My checkins ----

func (h *Handler) myCheckins(w http.ResponseWriter, r *http.Request) {
	p := httpx.ParsePageParams(r.URL.Query())
	subject, _ := authz.SubjectFrom(r.Context())
	list, total, err := h.store.PageMyCheckins(r.Context(), subject.ID,
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list my checkins", err)
		return
	}
	if list == nil {
		list = []MyCheckinView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

// ---- L2 semester summary ----

func (h *Handler) offeringSummary(w http.ResponseWriter, r *http.Request) {
	offeringID := queryInt(r.URL.Query(), "offering_id")
	if offeringID <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "offering_id is required")
		return
	}
	out, err := h.store.OfferingSummary(r.Context(), offeringID)
	if errors.Is(err, ErrCheckinNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "offering not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not build attendance summary", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, out)
}

func queryInt(q url.Values, key string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(q.Get(key)), 10, 64)
	return n
}

// statusLabel maps a record status to its Chinese display label.
func statusLabel(s string) string {
	switch s {
	case StatusPresent:
		return "出勤"
	case StatusLate:
		return "迟到"
	case StatusAbsent:
		return "缺勤"
	case StatusLeave:
		return "请假"
	}
	return s
}
