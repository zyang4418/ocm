package course

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// testSession builds a SessionView with the three display fields the grid
// exports and an explicit period range.
func testSession(periodStart, periodEnd int, courseName, teachingClass, teacher string) *SessionView {
	return &SessionView{
		Session:           Session{PeriodStart: periodStart, PeriodEnd: periodEnd},
		CourseName:        courseName,
		TeachingClassName: teachingClass,
		Teacher:           teacher,
	}
}

func TestBuildTimetableGrid(t *testing.T) {
	// Monday has a 2-period session (merged cell) and a free slot; Tuesday has
	// a single-period session.
	days := []TimetableDay{
		{
			Date: "2026-08-10", DayOfWeek: 1, RegimeName: "作息A",
			Slots: []TimetableSlot{
				{PeriodIndex: 1, StartTime: "08:00", EndTime: "08:45", Session: testSession(1, 2, "高等数学", "计科2301", "张三")},
				{PeriodIndex: 2, StartTime: "08:55", EndTime: "09:40", Session: testSession(1, 2, "高等数学", "计科2301", "张三")},
				{PeriodIndex: 3, StartTime: "09:50", EndTime: "10:35"},
			},
		},
		{
			Date: "2026-08-11", DayOfWeek: 2, RegimeName: "作息A",
			Slots: []TimetableSlot{
				{PeriodIndex: 2, StartTime: "08:55", EndTime: "09:40", Session: testSession(2, 2, "线性代数", "计科2302", "李四")},
			},
		},
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := buildTimetableGrid(f, days); err != nil {
		t.Fatalf("buildTimetableGrid: %v", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("encode workbook: %v", err)
	}

	got, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen workbook: %v", err)
	}
	defer func() { _ = got.Close() }()

	const sh = "教室周课表"
	check := func(cell, want string) {
		t.Helper()
		v, err := got.GetCellValue(sh, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if v != want {
			t.Errorf("%s = %q, want %q", cell, v, want)
		}
	}

	// Header row: corner + one column per day, three lines each.
	check("A1", "节次")
	check("B1", "周一\n08-10\n作息A")
	check("C1", "周二\n08-11\n作息A")

	// Period column: label + time range per row.
	check("A2", "第 1 节\n08:00-08:45")
	check("A3", "第 2 节\n08:55-09:40")
	check("A4", "第 3 节\n09:50-10:35")

	// Monday's 2-period session: value on the top-left cell; the covered cell
	// carries no value of its own (GetCellValue resolves merged ranges back to
	// the top-left, so the blank check uses GetRows, which does not).
	check("B2", "高等数学\n计科2301\n张三")
	rows, err := got.GetRows(sh)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) < 3 || len(rows[2]) < 2 || rows[2][1] != "" {
		t.Errorf("covered cell B3 should be blank, got %q", rows[2][1])
	}
	// Tuesday's single-period session lands on the period-2 row.
	check("C3", "线性代数\n计科2302\n李四")
	// Free cells stay blank.
	check("B4", "")
	check("C2", "")

	// Merge range is exactly the Monday session's span.
	merges, err := got.GetMergeCells(sh)
	if err != nil {
		t.Fatalf("GetMergeCells: %v", err)
	}
	if len(merges) != 1 {
		t.Fatalf("merge count = %d, want 1", len(merges))
	}
	start := merges[0].GetStartAxis()
	end := merges[0].GetEndAxis()
	if start != "B2" || end != "B3" {
		t.Errorf("merge = %s:%s, want B2:B3", start, end)
	}

	// Geometry: 4.25rem row height (51pt), header 45pt, fixed column widths.
	for _, tc := range []struct {
		row  int
		want float64
	}{{1, 45}, {2, 51}, {3, 51}, {4, 51}} {
		h, err := got.GetRowHeight(sh, tc.row)
		if err != nil {
			t.Fatalf("GetRowHeight(%d): %v", tc.row, err)
		}
		if h != tc.want {
			t.Errorf("row %d height = %v, want %v", tc.row, h, tc.want)
		}
	}
	for _, tc := range []struct {
		col  string
		want float64
	}{{"A", 12}, {"B", 18}, {"C", 18}} {
		w, err := got.GetColWidth(sh, tc.col)
		if err != nil {
			t.Fatalf("GetColWidth(%s): %v", tc.col, err)
		}
		if w != tc.want {
			t.Errorf("col %s width = %v, want %v", tc.col, w, tc.want)
		}
	}

	// Every grid cell (including merged interiors and free cells) is bordered.
	for _, cell := range []string{"A1", "B1", "C1", "A2", "B2", "B3", "C2", "A3", "C3", "A4", "B4", "C4"} {
		sid, err := got.GetCellStyle(sh, cell)
		if err != nil {
			t.Fatalf("GetCellStyle(%s): %v", cell, err)
		}
		st, err := got.GetStyle(sid)
		if err != nil {
			t.Fatalf("GetStyle(%s): %v", cell, err)
		}
		if len(st.Border) != 4 {
			t.Errorf("cell %s has %d borders, want 4", cell, len(st.Border))
		}
	}
}

func TestBuildTimetableGridEmpty(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := buildTimetableGrid(f, nil); err != nil {
		t.Fatalf("buildTimetableGrid(nil): %v", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("encode workbook: %v", err)
	}
	got, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen workbook: %v", err)
	}
	defer func() { _ = got.Close() }()
	if v, _ := got.GetCellValue("教室周课表", "A1"); v != "节次" {
		t.Errorf("empty grid corner = %q, want 节次", v)
	}
}

func TestTimetableExportFilename(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"A101", "2026-08-10", "2026-08-16", "A101教室周课表_2026-08-10~2026-08-16.xlsx"},
		{`../a:b*c`, "2026-08-10", "2026-08-16", "..abc教室周课表_2026-08-10~2026-08-16.xlsx"},
		{"", "2026-08-10", "2026-08-16", "教室周课表_2026-08-10~2026-08-16.xlsx"},
		{"阶梯教室. ", "2026-08-10", "2026-08-16", "阶梯教室教室周课表_2026-08-10~2026-08-16.xlsx"},
	} {
		if got := timetableExportFilename(tc.name, tc.from, tc.to); got != tc.want {
			t.Errorf("timetableExportFilename(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
