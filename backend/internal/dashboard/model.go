package dashboard

import (
	"ocm-backend/internal/booking"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/systemlog"
)

// Summary is the console homepage payload. Every section is optional: the
// handler includes a field only when the caller holds the permission that
// gates it (and the underlying query succeeded), so the frontend renders
// exactly the modules the user is allowed to see - absence means "hidden".
type Summary struct {
	// Date is the echoed "YYYY-MM-DD" school day the figures refer to
	// (supplied by the client so server/client timezone skew cannot shift
	// "today").
	Date string `json:"date"`

	// ClassroomTotal is the number of managed classrooms (classroom:read).
	ClassroomTotal *int64 `json:"classroomTotal,omitempty"`

	// TodaySessions is the day's course session count plus a preview of the
	// first sessions by period (course:read).
	TodaySessions *SessionsSection `json:"todaySessions,omitempty"`

	// SessionPeriods is today's occupancy histogram: for each period index
	// 1..max, how many sessions are running (a session spanning periods 1-2
	// counts in both slots). Filled alongside TodaySessions from the same
	// query, so it carries the same course:read gate (course:read).
	SessionPeriods []PeriodCount `json:"sessionPeriods,omitempty"`

	// BookingLoad is approved booking counts per day over the next 14 days,
	// zero-filled. Scope matches the caller: booking:approve sees every
	// booking, everyone else only their own. Visible to every authenticated
	// subject.
	BookingLoad []booking.DailyCount `json:"bookingLoad,omitempty"`

	// PendingBookings is the pending-approval queue (booking:approve).
	PendingBookings *BookingsSection `json:"pendingBookings,omitempty"`

	// OpenRepairs is the unresolved ticket queue, open or processing,
	// scoped like the repair list itself: repair:assign sees every ticket,
	// a plain repair:create only their own (repair:create OR repair:assign).
	OpenRepairs *RepairsSection `json:"openRepairs,omitempty"`

	// RecentLogs is the latest audit trail entries (log:read).
	RecentLogs []systemlog.LogView `json:"recentLogs,omitempty"`

	// MyBookings is the caller's own approved bookings starting at Date,
	// soonest first. Visible to every authenticated subject: a booking's
	// creator can always see their own reservation.
	MyBookings []booking.BookingView `json:"myBookings,omitempty"`
}

// SessionsSection carries the today KPI plus the leading sessions.
type SessionsSection struct {
	Total int                  `json:"total"`
	Items []course.SessionView `json:"items"`
}

// PeriodCount is one bar of the today period-occupancy histogram.
type PeriodCount struct {
	Period int `json:"period"`
	Count  int `json:"count"`
}

// BookingsSection carries a booking-queue KPI plus its newest entries.
type BookingsSection struct {
	Total int64                 `json:"total"`
	Items []booking.BookingView `json:"items"`
}

// RepairsSection carries the unresolved-ticket KPI plus its newest entries.
type RepairsSection struct {
	Total int64                  `json:"total"`
	Items []classroom.RepairView `json:"items"`
}
