// Package xlsx provides shared helpers for reading and writing .xlsx workbooks
// used by the import/export framework. Reading maps a header row to keyed maps
// (so column order in the file does not matter); writing produces a styled,
// frozen-header workbook streamed as an HTTP download.
package xlsx

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"
)

// MapRows reads the first worksheet of an xlsx document, treats the first row
// as a header, and returns each subsequent row as a map keyed by the lowercased,
// trimmed header name. Cells arrive as their formatted string value, so numeric
// and date cells are already stringified -- matching the importers' string-based
// validation. Fully blank rows are skipped.
func MapRows(data []byte) (headers []string, rows []map[string]string, err error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("打开 xlsx 失败：%w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, fmt.Errorf("xlsx 无工作表")
	}
	raw, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, fmt.Errorf("读取工作表失败：%w", err)
	}
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("工作表为空")
	}

	// First occurrence of each header wins; later duplicates are ignored so they
	// cannot shadow the first column of the same name.
	headerRow := raw[0]
	idx := make(map[string]int, len(headerRow))
	for i, h := range headerRow {
		key := strings.ToLower(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		if _, ok := idx[key]; !ok {
			idx[key] = i
			headers = append(headers, key)
		}
	}
	if len(idx) == 0 {
		return nil, nil, fmt.Errorf("表头为空")
	}

	rows = make([]map[string]string, 0, len(raw)-1)
	for _, rec := range raw[1:] {
		m := make(map[string]string, len(idx))
		empty := true
		for key, i := range idx {
			v := ""
			if i < len(rec) {
				v = strings.TrimSpace(rec[i])
			}
			if v != "" {
				empty = false
			}
			m[key] = v
		}
		if empty {
			continue
		}
		rows = append(rows, m)
	}
	return headers, rows, nil
}

// Has reports whether a required column is present among the headers.
func Has(headers []string, col string) bool {
	for _, h := range headers {
		if h == col {
			return true
		}
	}
	return false
}

// WriteExport writes an xlsx workbook with a single sheet to w. The first row is
// the bold, frozen header; each subsequent entry in rows is one record. Values
// keep their natural Go type via SetCellValue (numbers, dates, strings). The
// workbook is encoded to an in-memory buffer first so an encoding error can be
// reported before any HTTP headers are written.
func WriteExport(w http.ResponseWriter, filename, sheet string, headers []string, rows [][]any) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	name := sheet
	if name == "" {
		name = "Sheet1"
	}
	if name != "Sheet1" {
		if err := f.SetSheetName("Sheet1", name); err != nil {
			return fmt.Errorf("rename sheet: %w", err)
		}
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#E8E8E8"}},
	})
	if err != nil {
		return fmt.Errorf("create header style: %w", err)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(name, cell, h); err != nil {
			return fmt.Errorf("set header %q: %w", h, err)
		}
		if err := f.SetCellStyle(name, cell, cell, headerStyle); err != nil {
			return fmt.Errorf("style header %q: %w", h, err)
		}
		col, _ := excelize.ColumnNumberToName(i + 1)
		width := float64(len([]rune(h))) + 6
		if width < 12 {
			width = 12
		}
		_ = f.SetColWidth(name, col, col, width)
	}
	if err := f.SetPanes(name, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze header: %w", err)
	}

	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if err := f.SetCellValue(name, cell, v); err != nil {
				return fmt.Errorf("set cell row %d col %d: %w", r+2, c+1, err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return fmt.Errorf("encode xlsx: %w", err)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, err = w.Write(buf.Bytes())
	return err
}
