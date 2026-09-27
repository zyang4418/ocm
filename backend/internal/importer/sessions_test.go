package importer

import (
	"strings"
	"testing"

	"ocm-backend/internal/schedule"
)

// note 无域校验器（课次不经 CRUD 输入结构），VARCHAR(255) 上限在
// resolveSessionRow 内预检；这里直接测该行级检查。
func TestResolveSessionRowNoteTooLong(t *testing.T) {
	regimes := []schedule.Regime{{
		Name:           "Standard",
		EffectiveMonth: 9,
		EffectiveDay:   1,
		Periods:        []schedule.Period{{PeriodIndex: 1}, {PeriodIndex: 2}},
	}}
	rec := map[string]string{
		ColDate:          "2026-09-14",
		ColPeriodStart:   "1",
		ColPeriodEnd:     "2",
		ColClassroom:     "Room-101",
		ColCourse:        "Sample Course",
		ColSessionCode:   "TST101",
		ColTeachingClass: "Class A-241",
		ColSemester:      "2026-2027-1",
		ColNote:          strings.Repeat("注", 256),
	}
	rooms := map[string]int64{"Room-101": 1}
	offerings := map[string]offeringRef{"TST101|Class A-241|2026-2027-1": {id: 9, catalogName: "Sample Course"}}

	_, msg := resolveSessionRow(rec, rooms, offerings, regimes)
	if !strings.Contains(msg, "note") {
		t.Fatalf("超长 note 应报 note 长度错误，got %q", msg)
	}
}

// course 是显示列、code 是解析键：两者指向不同课程时按行拒绝，避免数据落到
// code 对应的课下而 preview 显示文件里的名字。
func TestResolveSessionRowNameMismatch(t *testing.T) {
	regimes := []schedule.Regime{{
		Name:           "Standard",
		EffectiveMonth: 9,
		EffectiveDay:   1,
		Periods:        []schedule.Period{{PeriodIndex: 1}},
	}}
	rec := map[string]string{
		ColDate:          "2026-09-14",
		ColPeriodStart:   "1",
		ColClassroom:     "Room-101",
		ColCourse:        "Other Course",
		ColSessionCode:   "TST101",
		ColTeachingClass: "Class A-241",
		ColSemester:      "2026-2027-1",
	}
	rooms := map[string]int64{"Room-101": 1}
	offerings := map[string]offeringRef{"TST101|Class A-241|2026-2027-1": {id: 9, catalogName: "Sample Course"}}

	_, msg := resolveSessionRow(rec, rooms, offerings, regimes)
	if !strings.Contains(msg, "课程名称与代码不符") || !strings.Contains(msg, "Other Course") {
		t.Fatalf("码名不符应报行错误，got %q", msg)
	}
}
