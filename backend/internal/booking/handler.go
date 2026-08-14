package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/schedule"
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

// RegisterRoutes mounts the booking endpoints. Reading (list/get) requires
// classroom:read; creating and cancelling requires classroom:book; approving
// or rejecting requires booking:approve (admin only).
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.ClassroomRead)(http.HandlerFunc(handler)))
	}
	book := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.ClassroomBook)(http.HandlerFunc(handler)))
	}
	approve := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.BookingApprove)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/bookings", read(h.list))
	mux.Handle("POST /api/bookings", book(h.create))
	mux.Handle("GET /api/bookings/export", read(h.export))
	mux.Handle("GET /api/bookings/{id}", read(h.get))
	mux.Handle("POST /api/bookings/{id}/cancel", book(h.cancel))
	mux.Handle("POST /api/bookings/{id}/review", approve(h.review))
}

// bookingFilter builds a ListFilter from the query string, shared by list and
// export so an export respects the same classroom/status/date filters the
// operator is currently viewing. from/to must be YYYY-MM-DD; anything else is
// rejected so MySQL does not silently coerce garbage like "abc" to 0000-00-00.
func bookingFilter(r *http.Request) (ListFilter, error) {
	q := r.URL.Query()
	classroomID, _ := strconv.ParseInt(q.Get("classroom_id"), 10, 64)
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)
	f := ListFilter{
		ClassroomID: classroomID,
		UserID:      userID,
		Status:      strings.TrimSpace(q.Get("status")),
	}
	if v := q.Get("from"); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return ListFilter{}, fmt.Errorf("invalid from (use YYYY-MM-DD)")
		}
		f.From = v
	}
	if v := q.Get("to"); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return ListFilter{}, fmt.Errorf("invalid to (use YYYY-MM-DD)")
		}
		f.To = v
	}
	return f, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	f, err := bookingFilter(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	list, total, err := h.store.PageBookings(r.Context(), f, httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list bookings")
		return
	}
	if list == nil {
		list = []BookingView{}
	}
	httpx.RespondPaged(w, list, total, p)
}

// export streams bookings as an xlsx download, respecting the same
// classroom/status/date filters as the list endpoint. The column layout matches
// the importer's expected headers so the file round-trips; status is included
// so an exported file can be re-imported as a faithful restore.
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	f, err := bookingFilter(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.store.List(r.Context(), f)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list bookings")
		return
	}
	headers := []string{"classroom", "username", "date", "period_start", "period_end", "status", "purpose"}
	rows := make([][]any, 0, len(list))
	for _, b := range list {
		rows = append(rows, []any{b.ClassroomName, b.Username, b.Date, b.PeriodStart, b.PeriodEnd, b.Status, b.Purpose})
	}
	if err := xlsx.WriteExport(w, "bookings.xlsx", "bookings", headers, rows); err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not export bookings")
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	v, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, ErrBookingNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "booking not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load booking")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, v)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in BookingInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := h.validateBooking(r.Context(), &in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	v, err := h.store.Create(r.Context(), in, subject.ID)
	if errors.Is(err, ErrClassroomConflict) {
		httpx.RespondError(w, http.StatusConflict, "classroom already booked for this date and period")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create booking")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, v)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	isAdmin := authz.Can(subject.Role, authz.BookingApprove)
	v, err := h.store.Cancel(r.Context(), id, subject.ID, isAdmin)
	switch {
	case errors.Is(err, ErrBookingNotFound):
		httpx.RespondError(w, http.StatusNotFound, "booking not found")
	case errors.Is(err, ErrForbidden):
		httpx.RespondError(w, http.StatusForbidden, "you can only cancel your own bookings")
	case errors.Is(err, ErrInvalidTransition):
		httpx.RespondError(w, http.StatusConflict, "booking cannot be cancelled in its current status")
	case err != nil:
		httpx.RespondError(w, http.StatusInternalServerError, "could not cancel booking")
	default:
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	var in ReviewInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Decision = strings.ToLower(strings.TrimSpace(in.Decision))
	if in.Decision != "approve" && in.Decision != "reject" {
		httpx.RespondError(w, http.StatusBadRequest, "decision must be 'approve' or 'reject'")
		return
	}
	v, err := h.store.Review(r.Context(), id, in.Decision)
	switch {
	case errors.Is(err, ErrBookingNotFound):
		httpx.RespondError(w, http.StatusNotFound, "booking not found")
	case errors.Is(err, ErrInvalidTransition):
		httpx.RespondError(w, http.StatusConflict, "booking is no longer pending")
	case errors.Is(err, ErrClassroomConflict):
		httpx.RespondError(w, http.StatusConflict, "classroom is no longer free for this period")
	case err != nil:
		httpx.RespondError(w, http.StatusInternalServerError, "could not review booking")
	default:
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

// validateBooking checks basic fields, that the classroom exists and is
// available, and that every period in [PeriodStart, PeriodEnd] exists in the
// active bell-time regime on the booking's date.
func (h *Handler) validateBooking(ctx context.Context, in *BookingInput) (string, bool) {
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
	in.Purpose = strings.TrimSpace(in.Purpose)
	if in.Purpose == "" {
		return "purpose is required", false
	}

	cr, err := h.classrooms.GetByID(ctx, in.ClassroomID)
	if errors.Is(err, classroom.ErrNotFound) {
		return "classroom not found", false
	}
	if err != nil {
		return "could not load classroom", false
	}
	if cr.Status != classroom.StatusAvailable {
		return "classroom is not available for booking", false
	}

	regimes, err := h.regimes.ListRegimes(ctx)
	if err != nil {
		return "could not validate against schedule regime", false
	}
	regime, ok := schedule.ActiveFor(regimes, date)
	if !ok {
		return "no schedule regime configured for this date", false
	}
	valid := schedule.PeriodIndexSet(regime)
	for i := in.PeriodStart; i <= in.PeriodEnd; i++ {
		if !valid[i] {
			return "period range is not valid for the active regime on this date", false
		}
	}
	return "", true
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
