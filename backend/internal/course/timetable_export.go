package course

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Weekly-grid xlsx export for the classroom timetable page. It replicates the
// browser table (web/src/pages/TimetablePage.jsx): days as columns, periods as
// rows, multi-period sessions merged into one cell, header row and period
// column centered both ways.

const (
	timetableSheetName    = "教室周课表"
	timetableFill         = "#F4F4F4" // --cds-layer-01
	timetableBorder       = "#E0E0E0" // --cds-border-subtle
	timetableRowHeight    = 51.0      // 4.25rem ≈ 68px ≈ 51pt, the browser's row height
	timetableHeaderHeight = 45.0      // fits the three-line day header (weekday + date + regime)
)

var weekdayNames = []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

// buildTimetableGrid renders the weekly grid into f, which must be a fresh
// workbook; the default "Sheet1" is renamed and the caller owns encoding and
// closing. Pure function of days — no database access.
func buildTimetableGrid(f *excelize.File, days []TimetableDay) error {
	if err := f.SetSheetName("Sheet1", timetableSheetName); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	sh := timetableSheetName

	// Union of period indices across the week, sorted ascending — mirrors the
	// periods memo in the browser (first slot wins for start/end times).
	periodMap := make(map[int]TimetableSlot)
	for _, d := range days {
		for _, s := range d.Slots {
			if _, ok := periodMap[s.PeriodIndex]; !ok {
				periodMap[s.PeriodIndex] = s
			}
		}
	}
	periods := make([]TimetableSlot, 0, len(periodMap))
	for _, s := range periodMap {
		periods = append(periods, s)
	}
	sort.Slice(periods, func(i, j int) bool { return periods[i].PeriodIndex < periods[j].PeriodIndex })

	border := []excelize.Border{
		{Type: "left", Color: timetableBorder, Style: 1},
		{Type: "right", Color: timetableBorder, Style: 1},
		{Type: "top", Color: timetableBorder, Style: 1},
		{Type: "bottom", Color: timetableBorder, Style: 1},
	}
	labelFill := excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{timetableFill}}

	// Header row + period column: centered both ways on grey, like the browser.
	labelStyle, err := f.NewStyle(&excelize.Style{
		Fill:      labelFill,
		Border:    border,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	if err != nil {
		return fmt.Errorf("create label style: %w", err)
	}
	// Session cell: left/top aligned with wrap, like the browser's cell.
	sessionStyle, err := f.NewStyle(&excelize.Style{
		Fill:      labelFill,
		Border:    border,
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "top", WrapText: true},
	})
	if err != nil {
		return fmt.Errorf("create session style: %w", err)
	}
	// Free cell: bordered only (the browser's "＋" is a click target, not data).
	emptyStyle, err := f.NewStyle(&excelize.Style{
		Border: border,
	})
	if err != nil {
		return fmt.Errorf("create empty style: %w", err)
	}

	// Header row: corner + one column per day.
	if err := f.SetCellValue(sh, "A1", "节次"); err != nil {
		return fmt.Errorf("set corner header: %w", err)
	}
	if err := f.SetCellStyle(sh, "A1", "A1", labelStyle); err != nil {
		return fmt.Errorf("style corner header: %w", err)
	}
	for i, d := range days {
		col, _ := excelize.ColumnNumberToName(i + 2)
		cell := col + "1"
		dayName := ""
		if d.DayOfWeek >= 1 && d.DayOfWeek <= 7 {
			dayName = weekdayNames[d.DayOfWeek-1]
		}
		dateLabel := d.Date
		if len(dateLabel) >= 10 {
			dateLabel = dateLabel[5:] // MM-DD, like d.date.slice(5) in the browser
		}
		label := dayName + "\n" + dateLabel
		if d.RegimeName != "" {
			label += "\n" + d.RegimeName
		}
		if err := f.SetCellValue(sh, cell, label); err != nil {
			return fmt.Errorf("set day header %d: %w", i, err)
		}
		if err := f.SetCellStyle(sh, cell, cell, labelStyle); err != nil {
			return fmt.Errorf("style day header %d: %w", i, err)
		}
	}
	if err := f.SetColWidth(sh, "A", "A", 12); err != nil {
		return fmt.Errorf("set corner column width: %w", err)
	}
	if len(days) > 0 {
		lastCol, _ := excelize.ColumnNumberToName(len(days) + 1)
		if err := f.SetColWidth(sh, "B", lastCol, 18); err != nil {
			return fmt.Errorf("set day column widths: %w", err)
		}
	}
	if err := f.SetPanes(sh, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze header: %w", err)
	}
	if err := f.SetRowHeight(sh, 1, timetableHeaderHeight); err != nil {
		return fmt.Errorf("set header row height: %w", err)
	}

	// Body rows. skip[di] counts rows already covered by a merged session cell
	// in day column di — the same conditional the browser renders with rowSpan
	// (TimetablePage.jsx: session && session.periodStart !== periodIndex → null).
	lastRow := 1 + len(periods)
	skip := make([]int, len(days))
	for ri, p := range periods {
		row := ri + 2
		cellA := "A" + strconv.Itoa(row)
		if err := f.SetCellValue(sh, cellA, fmt.Sprintf("第 %d 节\n%s-%s", p.PeriodIndex, p.StartTime, p.EndTime)); err != nil {
			return fmt.Errorf("set period label row %d: %w", row, err)
		}
		if err := f.SetCellStyle(sh, cellA, cellA, labelStyle); err != nil {
			return fmt.Errorf("style period label row %d: %w", row, err)
		}
		if err := f.SetRowHeight(sh, row, timetableRowHeight); err != nil {
			return fmt.Errorf("set row %d height: %w", row, err)
		}
		for di, d := range days {
			if ri < skip[di] {
				continue // covered by a merged cell from an earlier row
			}
			col, _ := excelize.ColumnNumberToName(di + 2)
			cell := col + strconv.Itoa(row)
			slot := slotFor(d, p.PeriodIndex)
			if slot == nil || slot.Session == nil {
				if err := f.SetCellStyle(sh, cell, cell, emptyStyle); err != nil {
					return fmt.Errorf("style free cell %s: %w", cell, err)
				}
				continue
			}
			ses := slot.Session
			if ses.PeriodStart != p.PeriodIndex {
				continue // session reaches here from an earlier row; merge covers it
			}
			span := ses.PeriodEnd - ses.PeriodStart + 1
			bottom := row + span - 1
			if bottom > lastRow {
				bottom = lastRow // stale span past the last grid row (regime changed)
			}
			lines := make([]string, 0, 3)
			for _, v := range []string{ses.CourseName, ses.TeachingClassName, ses.Teacher} {
				if v != "" {
					lines = append(lines, v)
				}
			}
			if err := f.SetCellValue(sh, cell, strings.Join(lines, "\n")); err != nil {
				return fmt.Errorf("set session cell %s: %w", cell, err)
			}
			// Style the whole range before merging: SetCellStyle stamps every
			// covered cell, and MergeCell only clears non-top-left values, never
			// styles — styling just the top-left would leave the covered cells
			// without borders.
			if err := f.SetCellStyle(sh, cell, col+strconv.Itoa(bottom), sessionStyle); err != nil {
				return fmt.Errorf("style session range %s:%s: %w", cell, col+strconv.Itoa(bottom), err)
			}
			if bottom > row {
				if err := f.MergeCell(sh, cell, col+strconv.Itoa(bottom)); err != nil {
					return fmt.Errorf("merge %s:%s: %w", cell, col+strconv.Itoa(bottom), err)
				}
			}
			skip[di] = ri + span
		}
	}
	return nil
}

// slotFor returns the slot of day at periodIndex, or nil when the day has no
// such period (same lookup as the browser).
func slotFor(day TimetableDay, periodIndex int) *TimetableSlot {
	for i := range day.Slots {
		if day.Slots[i].PeriodIndex == periodIndex {
			return &day.Slots[i]
		}
	}
	return nil
}

// populateTimetable adapts buildTimetableGrid to xlsx.WriteCustom's populate
// signature.
func populateTimetable(days []TimetableDay) func(*excelize.File) error {
	return func(f *excelize.File) error {
		return buildTimetableGrid(f, days)
	}
}

// timetableExportFilename builds the download filename for the weekly grid:
// classroom name + 教室周课表 + the date range shown on the page.
func timetableExportFilename(classroomName, from, to string) string {
	return sanitizeFilename(classroomName) + "教室周课表_" + from + "~" + to + ".xlsx"
}

// sanitizeFilename strips characters that are invalid in Windows filenames and
// control characters, and trims trailing dots/spaces.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			continue
		}
		if r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), ". ")
}
