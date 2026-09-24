package course

import "testing"

func TestSessionAuditLabel(t *testing.T) {
	cases := []struct {
		name string
		view SessionView
		want string
	}{
		{
			name: "period range",
			view: SessionView{
				Session:    Session{Date: "2026-09-25", PeriodStart: 3, PeriodEnd: 4},
				CourseName: "高等数学", ClassroomName: "A-302",
			},
			want: "高等数学 2026-09-25 第3–4节 A-302",
		},
		{
			name: "single period",
			view: SessionView{
				Session:    Session{Date: "2026-09-25", PeriodStart: 5, PeriodEnd: 5},
				CourseName: "大学英语", ClassroomName: "B-105",
			},
			want: "大学英语 2026-09-25 第5节 B-105",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionAuditLabel(tc.view); got != tc.want {
				t.Fatalf("sessionAuditLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}
