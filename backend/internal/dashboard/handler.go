package dashboard

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/booking"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/logging"
	"ocm-backend/internal/systemlog"
)

// Preview sizes: how many rows each dashboard list ships alongside its KPI.
const (
	sessionsPreview = 8
	pendingPreview  = 5
	repairPreview   = 5
	myBookingsLimit = 5
	myBookingsDays  = 14
	logsPreview     = 8
	bookingLoadDays = 14
)

// datePattern accepts exactly "YYYY-MM-DD". Any deviation is a 400 - the
// frontend always sends the client's local calendar date so the figures match
// what the user considers "today" regardless of server timezone.
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Handler aggregates read-only figures from the business stores into the
// console homepage summary. It owns no tables of its own - every query goes
// through the store that owns its table, reusing the exact list paths (and
// their scoping rules) the corresponding pages use.
type Handler struct {
	classrooms *classroom.Store
	repairs    *classroom.RepairStore
	courses    *course.Store
	books      *booking.Store
	logs       *systemlog.Store
}

func NewHandler(
	classrooms *classroom.Store,
	repairs *classroom.RepairStore,
	courses *course.Store,
	books *booking.Store,
	logs *systemlog.Store,
) *Handler {
	return &Handler{classrooms: classrooms, repairs: repairs, courses: courses, books: books, logs: logs}
}

// RegisterRoutes mounts the summary endpoint. It sits behind plain
// authenticate (any logged-in subject); section-level authorization is applied
// inside the handler against the request Subject, so low-privilege users get a
// shrunken payload rather than a 403.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/dashboard/summary", authenticate(http.HandlerFunc(h.summary)))
}

// visibility is which sections a subject unlocks. Extracted from the handler
// so the permission matrix is unit-testable without a database.
type visibility struct {
	Classrooms bool
	Sessions   bool
	Pending    bool
	Repairs    bool
	RepairAll  bool // repair:assign - see every ticket, not only own
	Logs       bool
}

// visibilityFor computes the section matrix from the effective permission set.
func visibilityFor(s authz.Subject) visibility {
	return visibility{
		Classrooms: s.Has(authz.ClassroomRead),
		Sessions:   s.Has(authz.CourseRead),
		Pending:    s.Has(authz.BookingApprove),
		Repairs:    s.Has(authz.RepairCreate) || s.Has(authz.RepairAssign),
		RepairAll:  s.Has(authz.RepairAssign),
		Logs:       s.Has(authz.LogRead),
	}
}

// @Summary      Console homepage summary (permission-gated sections)
// @Description  Each section is present only when the caller holds the
// @Description  permission that gates it — absence means "hidden".
// @Tags         dashboard
// @Produce      json
// @Param        date query string true "school day (Y-M-D)"
// @Success      200 {object} Summary "sectioned summary"
// @Failure      400 {object} httpx.ErrorResponse "date must be YYYY-MM-DD"
// @Failure      401 {object} httpx.ErrorResponse "not authenticated"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/dashboard/summary [get]
func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if !datePattern.MatchString(date) {
		httpx.RespondError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	vis := visibilityFor(subject)
	ctx := r.Context()
	out := Summary{Date: date}

	// Each section is fetched concurrently and writes to its own field of
	// out (distinct fields, no shared state). A failing section logs and is
	// simply omitted - the homepage degrades gracefully instead of 500ing
	// over one broken metric.
	var wg sync.WaitGroup
	load := func(section string, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	if vis.Classrooms {
		load("classrooms", func() {
			_, total, err := h.classrooms.PageClassrooms(ctx, "", dbutil.Pagination{Limit: 1})
			if logSection("classrooms", err) {
				return
			}
			out.ClassroomTotal = &total
		})
	}

	if vis.Sessions {
		load("sessions", func() {
			list, err := h.courses.ListSessions(ctx, 0, 0, date, date)
			if logSection("sessions", err) {
				return
			}
			total := len(list)
			// The full-day list feeds both the preview and the period
			// histogram before truncation.
			histogram := periodHistogram(list)
			if len(list) > sessionsPreview {
				list = list[:sessionsPreview]
			}
			out.TodaySessions = &SessionsSection{Total: total, Items: list}
			out.SessionPeriods = histogram
		})
	}

	// 14-day approved-booking load. Approvers see the school-wide line,
	// everyone else their own - the same scoping the repair section applies.
	load("bookingLoad", func() {
		f := booking.ListFilter{Status: booking.StatusApproved}
		if !vis.Pending {
			f.UserID = subject.ID
		}
		counts, err := h.books.CountByDate(ctx, f, date, addDays(date, bookingLoadDays-1))
		if logSection("bookingLoad", err) {
			return
		}
		out.BookingLoad = zeroFillDaily(counts, date, bookingLoadDays)
	})

	if vis.Pending {
		load("pendingBookings", func() {
			// No date window: the review queue is every pending request,
			// past dates included, matching what /bookings shows by default.
			list, total, err := h.books.PageBookings(ctx, booking.ListFilter{Status: booking.StatusPending}, "",
				dbutil.Pagination{Limit: pendingPreview})
			if logSection("pendingBookings", err) {
				return
			}
			out.PendingBookings = &BookingsSection{Total: total, Items: list}
		})
	}

	if vis.Repairs {
		load("openRepairs", func() {
			list, total, err := h.repairs.Page(ctx,
				classroom.RepairFilter{Statuses: []string{classroom.RepairStatusOpen, classroom.RepairStatusProcessing}},
				"", subject.ID, vis.RepairAll, dbutil.Pagination{Limit: repairPreview})
			if logSection("openRepairs", err) {
				return
			}
			out.OpenRepairs = &RepairsSection{Total: total, Items: list}
		})
	}

	if vis.Logs {
		load("recentLogs", func() {
			list, _, err := h.logs.PageLogs(ctx, systemlog.LogFilter{}, "", dbutil.Pagination{Limit: logsPreview})
			if logSection("recentLogs", err) {
				return
			}
			out.RecentLogs = list
		})
	}

	// Own approved bookings for everyone: the creator may always review their
	// reservations regardless of list permissions.
	load("myBookings", func() {
		list, _, err := h.books.PageBookings(ctx, booking.ListFilter{
			UserID: subject.ID, Status: booking.StatusApproved, From: date, To: addDays(date, myBookingsDays),
		}, "", dbutil.Pagination{})
		if logSection("myBookings", err) {
			return
		}
		sortBookingsAscending(list)
		if len(list) > myBookingsLimit {
			list = list[:myBookingsLimit]
		}
		out.MyBookings = list
	})

	wg.Wait()
	httpx.RespondJSON(w, http.StatusOK, out)
}

// logSection reports whether the section should be skipped and logs the
// failure otherwise (dashboard sections fail soft).
func logSection(name string, err error) bool {
	if err == nil {
		return false
	}
	logging.L.Error("dashboard section failed", "section", name, "err", err)
	return true
}

// sortBookingsAscending orders soonest-first; the store returns newest-first
// for list pages, but "my upcoming bookings" reads best ascending.
func sortBookingsAscending(list []booking.BookingView) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Date != list[j].Date {
			return list[i].Date < list[j].Date
		}
		if list[i].PeriodStart != list[j].PeriodStart {
			return list[i].PeriodStart < list[j].PeriodStart
		}
		return list[i].ID < list[j].ID
	})
}

// periodHistogram expands every session across its [PeriodStart, PeriodEnd]
// range and returns one slot per period 1..max, so the chart's x-axis is the
// full school day. A session spanning two periods counts in both slots, which
// is what "how busy is period N" means. Returns nil when there are no sessions
// (the section is omitted rather than shipping an all-zero axis).
func periodHistogram(list []course.SessionView) []PeriodCount {
	maxPeriod := 0
	for _, ses := range list {
		if ses.PeriodEnd > maxPeriod {
			maxPeriod = ses.PeriodEnd
		}
	}
	if maxPeriod == 0 {
		return nil
	}
	counts := make([]PeriodCount, maxPeriod)
	for p := 1; p <= maxPeriod; p++ {
		counts[p-1] = PeriodCount{Period: p}
	}
	for _, ses := range list {
		for p := ses.PeriodStart; p <= ses.PeriodEnd && p <= maxPeriod; p++ {
			counts[p-1].Count++
		}
	}
	return counts
}

// zeroFillDaily spreads sparse per-day counts over a continuous n-day window
// starting at from, so the trend line has a point for every day (flat zero
// days included) instead of gaps.
func zeroFillDaily(counts []booking.DailyCount, from string, n int) []booking.DailyCount {
	byDate := make(map[string]int64, len(counts))
	for _, dc := range counts {
		byDate[dc.Date] = dc.Count
	}
	out := make([]booking.DailyCount, n)
	for i := 0; i < n; i++ {
		d := addDays(from, i)
		out[i] = booking.DailyCount{Date: d, Count: byDate[d]}
	}
	return out
}

// addDays shifts a "YYYY-MM-DD" string forward n days, staying in string form
// to match how dates flow through the booking/course stores.
func addDays(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}
