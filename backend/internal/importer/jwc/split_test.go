package jwc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ocm-backend/internal/schedule"
	"ocm-backend/internal/xlsx"
)

// allPeriods 构造含 1..10 节的作息制度，供 Split 预校验通过。
func allPeriods() []schedule.Period {
	ps := make([]schedule.Period, 10)
	for i := 0; i < 10; i++ {
		ps[i] = schedule.Period{PeriodIndex: i + 1}
	}
	return ps
}

// TestSplitEndToEnd 用脱敏样本（testdata/sample_timetable.xlsx）跑完整拆分，断言关键不变量：
// 6 张 xlsx 非空且表头匹配各 importer 契约、教学班名 ≤64 且回退时 note 非空、
// 开课教师非空、sessions 同一 (教室+日期) 内节次区间不重叠且日期合法。
// 计数断言（Classrooms 等）基于样本重算，样本生成见 testdata/gen_sample.go 与 testdata/README.md。
func TestSplitEndToEnd(t *testing.T) {
	path := filepath.Join("testdata", "sample_timetable.xlsx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取样本失败（应已入库 testdata/）：%v", err)
	}

	week1Monday := time.Date(2024, 9, 2, 0, 0, 0, 0, time.UTC) // 2024-09-02 周一
	regimes := []schedule.Regime{
		{Name: "测试作息", EffectiveMonth: 1, EffectiveDay: 1, Periods: allPeriods()},
	}

	res, err := Split(data, "2024-2025-1", week1Monday, regimes)
	if err != nil {
		t.Fatalf("Split 失败：%v", err)
	}
	st := res.Stats
	t.Logf("stats: %+v", st)
	t.Logf("warnings(%d):", len(st.Warnings))
	for i, w := range st.Warnings {
		if i >= 10 {
			t.Logf("  ...（其余 %d 条省略）", len(st.Warnings)-10)
			break
		}
		t.Logf("  %s", w)
	}

	// 计数（样本重算值；sample_timetable.xlsx 为 committed 二进制，确定性）。
	checks := []struct {
		name      string
		got, want int
	}{
		{"Classrooms", st.Classrooms, 21},
		{"CatalogCourses", st.CatalogCourses, 22},
		{"AdminClasses", st.AdminClasses, 62},
		{"TeachingClasses", st.TeachingClasses, 19},
		{"SkippedEmptyAdmin", st.SkippedEmptyAdmin, 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d，期望 %d", c.name, c.got, c.want)
		}
	}

	// 平行教学班：样本含 1 对（2 个序号均跳过）。
	if st.SkippedParallel != 2 {
		t.Errorf("SkippedParallel = %d，期望 2", st.SkippedParallel)
	}
	// 课次：节次区间化后一个槽位每周一条（不再逐节展开），样本重算为 241。
	if st.Sessions < 230 || st.Sessions > 260 {
		t.Errorf("Sessions = %d，期望 230~260", st.Sessions)
	}

	// 6 张 xlsx 均非空且可被 MapRows 回读，表头匹配各 importer 契约。
	files := []struct {
		name string
		data []byte
		want []string
	}{
		{"classrooms", res.Files.Classrooms, []string{"name", "building", "capacity", "type", "floor", "campus", "status", "description"}},
		{"catalog", res.Files.Catalog, []string{"name", "code", "credits", "total_hours", "category", "exam_type", "description"}},
		{"admin_classes", res.Files.AdminClasses, []string{"grade", "name", "note"}},
		{"teaching_classes", res.Files.TeachingClasses, []string{"name", "note", "admin_grade", "admin_name"}},
		{"offerings", res.Files.Offerings, []string{"course", "teaching_class", "semester", "teacher", "course_seq", "teacher_id", "teacher_title", "college", "max_students", "requirement", "weekly_hours", "note"}},
		{"sessions", res.Files.Sessions, []string{"date", "period_start", "period_end", "classroom", "course", "teaching_class", "semester", "note"}},
	}
	for _, f := range files {
		if len(f.data) == 0 {
			t.Errorf("%s.xlsx 为空", f.name)
			continue
		}
		headers, _, err := xlsx.MapRows(f.data)
		if err != nil {
			t.Errorf("%s.xlsx 回读失败：%v", f.name, err)
			continue
		}
		for _, h := range f.want {
			if !xlsx.Has(headers, h) {
				t.Errorf("%s.xlsx 缺少表头 %q", f.name, h)
			}
		}
	}

	// 教学班名 ≤64 字符（rune 计），且回退命名时 note 非空。
	_, tcRows, _ := xlsx.MapRows(res.Files.TeachingClasses)
	nameNote := map[string]string{}
	for _, r := range tcRows {
		if existing, ok := nameNote[r["name"]]; !ok || existing == "" {
			nameNote[r["name"]] = r["note"]
		}
	}
	fallback := 0
	for name, note := range nameNote {
		if len([]rune(name)) > 64 {
			t.Errorf("教学班名超 64 字符：%q（%d）", name, len([]rune(name)))
		}
		if len([]rune(name)) > 64 || strings.Contains(name, "等") {
			if note == "" {
				t.Errorf("回退命名但 note 为空：%q", name)
			}
			fallback++
		}
	}
	if fallback == 0 {
		t.Errorf("未见任何回退命名，期望样本含 ≥1 个回退教学班")
	}
	t.Logf("回退命名教学班数：%d", fallback)

	// 开课教师非空（NoTeacherFilled 行除外：teacher 填 "未安排"）。
	_, offRows, _ := xlsx.MapRows(res.Files.Offerings)
	for i, r := range offRows {
		if strings.TrimSpace(r["teacher"]) == "" {
			t.Errorf("offerings 第 %d 行教师为空", i+2)
		}
		if r["course"] == "" || r["teaching_class"] == "" || r["semester"] == "" {
			t.Errorf("offerings 第 %d 行关键字段为空", i+2)
		}
	}

	// sessions 同一 (教室+日期) 内节次区间不得重叠；日期格式合法；节次 1..10。
	seen := map[string][][2]int{}
	_, sessRows, _ := xlsx.MapRows(res.Files.Sessions)
	for i, r := range sessRows {
		ps, err1 := strconv.Atoi(r["period_start"])
		pe, err2 := strconv.Atoi(r["period_end"])
		if err1 != nil || err2 != nil || ps < 1 || pe < ps || pe > 10 {
			t.Errorf("sessions 第 %d 行节次区间非法：%q-%q", i+2, r["period_start"], r["period_end"])
		}
		key := r["classroom"] + "|" + r["date"]
		for _, prev := range seen[key] {
			if ps <= prev[1] && prev[0] <= pe {
				t.Errorf("sessions 第 %d 行节次区间与同教室同时段课次重叠：%s", i+2, key)
				break
			}
		}
		seen[key] = append(seen[key], [2]int{ps, pe})
		if _, err := time.Parse("2006-01-02", r["date"]); err != nil {
			t.Errorf("sessions 第 %d 行日期非法：%q", i+2, r["date"])
		}
	}
}

// TestParsePeriods 单测节次区间解析（连上多节保持为一个区间）。
func TestParsePeriods(t *testing.T) {
	cases := []struct {
		in       string
		wantFrom int
		wantTo   int
	}{
		{"3-4", 3, 4},
		{"1-4", 1, 4},
		{"9-10", 9, 10},
		{"1-1", 1, 1},
		{"2-2", 2, 2},
	}
	for _, c := range cases {
		from, to, err := parsePeriods(c.in)
		if err != nil {
			t.Errorf("parsePeriods(%q) err: %v", c.in, err)
			continue
		}
		if from != c.wantFrom || to != c.wantTo {
			t.Errorf("parsePeriods(%q) = (%d,%d), want (%d,%d)", c.in, from, to, c.wantFrom, c.wantTo)
		}
	}
	if _, _, err := parsePeriods(""); err == nil {
		t.Error("parsePeriods(\"\") 期望错误")
	}
}

// TestExpandWeeks 单测起止周展开（含单/双奇偶过滤）。
func TestExpandWeeks(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"[1-8]", []int{1, 2, 3, 4, 5, 6, 7, 8}},
		{"[1-4],[6-17]", []int{1, 2, 3, 4, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}},
		{"3-3", []int{3}},
		{"[1-5],9", []int{1, 2, 3, 4, 5, 9}},
		{"[1-3]单", []int{1, 3}}, // 奇数周
		{"[4-6]双", []int{4, 6}}, // 偶数周
		{"[1-3]单,[4-6]双,[7-9]", []int{1, 3, 4, 6, 7, 8, 9}},
	}
	for _, c := range cases {
		got, err := expandWeeks(c.in)
		if err != nil {
			t.Errorf("expandWeeks(%q) err: %v", c.in, err)
			continue
		}
		if !equal(got, c.want) {
			t.Errorf("expandWeeks(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := expandWeeks(""); err == nil {
		t.Error("expandWeeks(\"\") 期望错误")
	}
}

// TestTeachingClassName 单测教学班命名（区间压缩 + 回退）。
func TestTeachingClassName(t *testing.T) {
	// 连续 -> 压缩。
	name, note := teachingClassName([]string{"机电241", "机电242", "机电243", "机电244", "机电245"})
	if name != "机电241~245" || note != "" {
		t.Errorf("压缩命名 = %q note=%q，期望 机电241~245", name, note)
	}
	// 多组连续。
	name, _ = teachingClassName([]string{"人工智能241", "人工智能242", "电子241", "电子242"})
	if name != "人工智能241~242,电子241~242" {
		t.Errorf("多组命名 = %q", name)
	}
	// 断裂 -> 原样。
	name, _ = teachingClassName([]string{"机电241", "机电243"})
	if name != "机电241,机电243" {
		t.Errorf("断裂命名 = %q", name)
	}
	// 单个 -> 原样。
	name, _ = teachingClassName([]string{"机电241"})
	if name != "机电241" {
		t.Errorf("单班命名 = %q", name)
	}
	// 超长 -> 回退「首班,次班等N班」+ note。用互异前缀避免区间压缩后仍较短。
	many := []string{
		"机电241", "电气241", "自动241", "电子241", "通信241", "计算机241",
		"软工241", "网工241", "信安241", "人工智能241", "数据241", "物联241",
	}
	name, note = teachingClassName(many)
	if !strings.Contains(name, "等12班") {
		t.Errorf("回退命名 = %q，期望含「等12班」", name)
	}
	if len([]rune(name)) > 64 {
		t.Errorf("回退命名仍超 64：%q", name)
	}
	if note == "" || !strings.Contains(note, "机电241") {
		t.Errorf("回退 note = %q，期望含完整成员", note)
	}
}

// TestGradeFor 单测年级推导。
func TestGradeFor(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"机电241", "", "2024"},
		{"机电221", "", "2022"},
		{"工设UC-241", "", "2024"},
		{"机电241", "2025", "2025"}, // 年级列优先
	}
	for _, c := range cases {
		if got := gradeFor(c.name, c.src); got != c.want {
			t.Errorf("gradeFor(%q,%q) = %q, want %q", c.name, c.src, got, c.want)
		}
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
