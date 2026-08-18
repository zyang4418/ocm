package dashboard

import (
	"testing"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/booking"
	"ocm-backend/internal/course"
)

func subject(perms ...string) authz.Subject {
	s := authz.Subject{Permissions: map[string]bool{}}
	for _, p := range perms {
		s.Permissions[p] = true
	}
	return s
}

// TestVisibilityFor locks the section/permission matrix: a section unlocks iff
// its gating permission is held (directly or via "*"), repairs unlocks on
// either repair permission, and RepairAll only on repair:assign.
func TestVisibilityFor(t *testing.T) {
	cases := []struct {
		name  string
		perms []string
		want  visibility
	}{
		{"plain user", nil, visibility{}},
		{
			"teacher",
			[]string{authz.CourseRead, authz.RepairCreate, authz.ClassroomRead},
			visibility{Classrooms: true, Sessions: true, Repairs: true},
		},
		{
			"repair assigner",
			[]string{authz.RepairAssign},
			visibility{Repairs: true, RepairAll: true},
		},
		{
			"admin",
			[]string{authz.Wildcard},
			visibility{Classrooms: true, Sessions: true, Pending: true, Repairs: true, RepairAll: true, Logs: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := visibilityFor(subject(tc.perms...)); got != tc.want {
				t.Fatalf("visibilityFor(%v) = %+v, want %+v", tc.perms, got, tc.want)
			}
		})
	}
}

func TestAddDays(t *testing.T) {
	cases := map[string]struct {
		in   string
		n    int
		want string
	}{
		"same month":     {"2026-08-18", 3, "2026-08-21"},
		"month rollover": {"2026-08-30", 3, "2026-09-02"},
		"year rollover":  {"2026-12-31", 1, "2027-01-01"},
		"leap february":  {"2028-02-27", 2, "2028-02-29"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := addDays(tc.in, tc.n); got != tc.want {
				t.Fatalf("addDays(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

// TestPeriodHistogram locks the today-occupancy expansion: a session counts in
// every period it spans, slots run 1..max with zeros preserved, and an empty
// day yields nil (section omitted).
func TestPeriodHistogram(t *testing.T) {
	sv := func(ps, pe int) course.SessionView {
		return course.SessionView{Session: course.Session{PeriodStart: ps, PeriodEnd: pe}}
	}
	cases := []struct {
		name string
		in   []course.SessionView
		want []PeriodCount
	}{
		{"empty day", nil, nil},
		{
			"single period",
			[]course.SessionView{sv(2, 2)},
			[]PeriodCount{{1, 0}, {2, 1}},
		},
		{
			"spanning session counts in each slot",
			[]course.SessionView{sv(1, 2), sv(2, 3), sv(2, 2)},
			[]PeriodCount{{1, 1}, {2, 3}, {3, 1}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := periodHistogram(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("periodHistogram(%v) = %+v, want %+v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("periodHistogram(%v)[%d] = %+v, want %+v", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestZeroFillDaily locks the trend-window assembly: every day of the window
// appears exactly once, sparse input keeps its counts, missing days become
// zero, and out-of-window rows are dropped.
func TestZeroFillDaily(t *testing.T) {
	counts := []booking.DailyCount{
		{Date: "2026-08-19", Count: 2},
		{Date: "2026-08-22", Count: 5},
		{Date: "2025-01-01", Count: 99}, // outside the window
	}
	got := zeroFillDaily(counts, "2026-08-18", 4)
	want := []booking.DailyCount{
		{Date: "2026-08-18", Count: 0},
		{Date: "2026-08-19", Count: 2},
		{Date: "2026-08-20", Count: 0},
		{Date: "2026-08-21", Count: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("zeroFillDaily returned %d days, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("zeroFillDaily[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
