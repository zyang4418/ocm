package course

import (
	"testing"

	"ocm-backend/internal/booking"
	"ocm-backend/internal/schedule"
)

// oneRegime builds a 4-period regime effective from January 1st, so it is
// active for every date used in these tests.
func oneRegime() schedule.Regime {
	r := schedule.Regime{Name: "作息A", EffectiveMonth: 1, EffectiveDay: 1}
	for i := 1; i <= 4; i++ {
		r.Periods = append(r.Periods, schedule.Period{
			PeriodIndex: i, StartTime: "08:00", EndTime: "08:45",
		})
	}
	return r
}

func sessionView(date string, periodStart, periodEnd int) SessionView {
	return SessionView{
		Session: Session{Date: date, PeriodStart: periodStart, PeriodEnd: periodEnd},
	}
}

func bookingView(date string, periodStart, periodEnd int, purpose string) booking.BookingView {
	return booking.BookingView{
		Booking: booking.Booking{Date: date, PeriodStart: periodStart, PeriodEnd: periodEnd, Purpose: purpose},
	}
}

func TestBuildTimetableDays(t *testing.T) {
	regimes := []schedule.Regime{oneRegime()}
	sessions := []SessionView{sessionView("2026-08-10", 1, 2)}
	// One booking beside the session on the same day, and one on the next day;
	// a third overlaps the session's range, which the conflict checks prevent
	// in production - the session must win defensively.
	bookings := []booking.BookingView{
		bookingView("2026-08-10", 3, 4, "社团活动"),
		bookingView("2026-08-11", 1, 1, "答疑"),
		bookingView("2026-08-10", 2, 2, "重叠"),
	}

	days, err := buildTimetableDays(regimes, sessions, bookings, "2026-08-10", "2026-08-11")
	if err != nil {
		t.Fatalf("buildTimetableDays: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("day count = %d, want 2", len(days))
	}

	monday, tuesday := days[0], days[1]
	if len(monday.Slots) != 4 || len(tuesday.Slots) != 4 {
		t.Fatalf("slot counts = %d/%d, want 4/4", len(monday.Slots), len(tuesday.Slots))
	}
	slot := func(d TimetableDay, period int) TimetableSlot {
		for _, s := range d.Slots {
			if s.PeriodIndex == period {
				return s
			}
		}
		t.Fatalf("period %d not found", period)
		return TimetableSlot{}
	}

	// Monday: the session covers periods 1-2 (session wins over the
	// overlapping booking), the booking covers periods 3-4.
	if s := slot(monday, 1); s.Session == nil || s.Booking != nil {
		t.Errorf("monday period 1: session=%v booking=%v, want session only", s.Session != nil, s.Booking != nil)
	}
	if s := slot(monday, 2); s.Session == nil || s.Booking != nil {
		t.Errorf("monday period 2: session=%v booking=%v, want session only (overlapping booking must lose)", s.Session != nil, s.Booking != nil)
	}
	if s := slot(monday, 3); s.Session != nil || s.Booking == nil || s.Booking.Purpose != "社团活动" {
		t.Errorf("monday period 3: want booking 社团活动, got session=%v booking=%v", s.Session != nil, s.Booking != nil)
	}
	if s := slot(monday, 4); s.Session != nil || s.Booking == nil {
		t.Errorf("monday period 4: want booking, got session=%v booking=%v", s.Session != nil, s.Booking != nil)
	}
	// Both covered periods of the session point at the same record.
	if slot(monday, 1).Session.ID != slot(monday, 2).Session.ID {
		t.Errorf("monday periods 1 and 2 should share one session")
	}

	// Tuesday: booking on period 1, periods 2-4 free.
	if s := slot(tuesday, 1); s.Session != nil || s.Booking == nil || s.Booking.Purpose != "答疑" {
		t.Errorf("tuesday period 1: want booking 答疑, got session=%v booking=%v", s.Session != nil, s.Booking != nil)
	}
	for _, p := range []int{2, 3, 4} {
		if s := slot(tuesday, p); s.Session != nil || s.Booking != nil {
			t.Errorf("tuesday period %d should be free", p)
		}
	}
}

func TestBuildTimetableDaysNoRegime(t *testing.T) {
	// Without any regime the days carry no slots, but sessions and bookings are
	// not an error (the browser renders an empty grid).
	days, err := buildTimetableDays(nil, nil, nil, "2026-08-10", "2026-08-11")
	if err != nil {
		t.Fatalf("buildTimetableDays: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("day count = %d, want 2", len(days))
	}
	for i, d := range days {
		if len(d.Slots) != 0 {
			t.Errorf("day %d slot count = %d, want 0", i, len(d.Slots))
		}
	}
}

func TestBuildTimetableDaysBadDate(t *testing.T) {
	if _, err := buildTimetableDays(nil, nil, nil, "not-a-date", "2026-08-11"); err == nil {
		t.Errorf("invalid from date should be an error")
	}
	if _, err := buildTimetableDays(nil, nil, nil, "2026-08-10", "not-a-date"); err == nil {
		t.Errorf("invalid to date should be an error")
	}
}
