package docx

import (
	"strings"

	"github.com/beevik/etree"
)

// Check returns the filled checkmark ("√") for a selected rating slot, or an
// empty string.
func Check(selected bool) string {
	if selected {
		return "√"
	}
	return ""
}

// Checkbox returns a filled (☑) or empty (☐) ballot box.
func Checkbox(selected bool) string {
	if selected {
		return "☑"
	}
	return "☐"
}

// LineLimit describes how a text block is constrained in the template: either a
// uniform max chars-per-line across max lines, or an explicit per-line width
// list (used when the template's rows have differing widths).
type LineLimit struct {
	MaxLines        int
	MaxCharsPerLine int
	LineCharLimits  []int
}

// limitWidths returns the per-line width list implied by the limit.
func limitWidths(limit LineLimit) []int {
	if len(limit.LineCharLimits) > 0 {
		out := make([]int, len(limit.LineCharLimits))
		copy(out, limit.LineCharLimits)
		return out
	}
	if limit.MaxLines <= 0 || limit.MaxCharsPerLine <= 0 {
		return nil
	}
	out := make([]int, limit.MaxLines)
	for i := range out {
		out[i] = limit.MaxCharsPerLine
	}
	return out
}

// WrapTextToLines breaks text into lines honoring the limit, matching the
// legacy exporter's code-point slicing. Explicit newlines start a new line;
// anything beyond the allowed line count is dropped. With no usable limit the
// text is returned as a single line.
func WrapTextToLines(text string, limit LineLimit) []string {
	widths := limitWidths(limit)
	if len(widths) == 0 {
		if text == "" {
			return []string{""}
		}
		return []string{text}
	}

	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var out []string

	for _, rawLine := range strings.Split(normalized, "\n") {
		if len(out) >= len(widths) {
			break
		}
		if rawLine == "" {
			out = append(out, "")
			continue
		}
		remaining := []rune(rawLine)
		for len(remaining) > 0 && len(out) < len(widths) {
			width := widths[len(out)]
			if width < 1 {
				// Defensive: a zero/negative width has no sensible wrap;
				// emit the rest of the line and stop.
				out = append(out, string(remaining))
				break
			}
			if width > len(remaining) {
				width = len(remaining)
			}
			out = append(out, string(remaining[:width]))
			remaining = remaining[width:]
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// padUnderlinedSlot pads value with trailing spaces up to slotWidth (bounded by
// maxPadding) so a short value visually fills the template's underlined slot.
func padUnderlinedSlot(value, slotText string, maxPadding int) string {
	if value == "" {
		return slotText
	}
	slotWidth := len([]rune(slotText))
	padding := slotWidth - len([]rune(value))
	if padding < 0 {
		padding = 0
	}
	if maxPadding > 0 && padding > maxPadding {
		padding = maxPadding
	}
	return value + strings.Repeat(" ", padding)
}

// SetUnderlinedSlots fills consecutive underlined runs of a paragraph with the
// given values. Runs that share one underline (split into multiple w:r by
// Word's proofing) are treated as a single slot: the first run of each group
// receives the value, the rest are emptied. maxPadding caps how far a short
// value is padded (0 = pad to the full slot width).
func SetUnderlinedSlots(p *etree.Element, values []string, maxPadding int) {
	valueIndex := 0
	var group []*etree.Element

	flush := func() {
		if len(group) == 0 {
			return
		}
		var sb strings.Builder
		for _, r := range group {
			sb.WriteString(RunText(r))
		}
		slotText := sb.String()
		if valueIndex < len(values) {
			SetRunText(group[0], padUnderlinedSlot(values[valueIndex], slotText, maxPadding))
			valueIndex++
		}
		for _, r := range group[1:] {
			SetRunText(r, "")
		}
		group = nil
	}

	for _, t := range p.Child {
		r, ok := t.(*etree.Element)
		if !ok || !IsWord(r, "r") {
			flush()
			continue
		}
		if IsUnderlinedRun(r) {
			group = append(group, r)
		} else {
			flush()
		}
	}
	flush()
}

// FillUnderlinedSlots fills the underlined slots of the body paragraph whose
// text starts with prefix.
func FillUnderlinedSlots(d *Document, prefix string, values []string) error {
	p, err := d.ParagraphByPrefix(prefix)
	if err != nil {
		return err
	}
	SetUnderlinedSlots(p, values, 0)
	return nil
}

// cellAt returns the w:tc at (row, col) of table, or nil if out of range.
func cellAt(table *etree.Element, row, col int) *etree.Element {
	rows := Children(table, "tr")
	if row < 0 || row >= len(rows) {
		return nil
	}
	cells := Children(rows[row], "tc")
	if col < 0 || col >= len(cells) {
		return nil
	}
	return cells[col]
}

// SetCellText replaces a cell's content with a single paragraph carrying text,
// preserving the cell's first run and paragraph properties.
func SetCellText(table *etree.Element, row, col int, text string) {
	cell := cellAt(table, row, col)
	if cell == nil {
		return
	}
	rPr := FirstRunProperties(cell)
	pPr := FirstParagraphProperties(cell)
	clearChildren(cell, "tcPr")
	p := WordElement("p")
	if pPr != nil {
		p.AddChild(pPr)
	}
	cell.AddChild(p)
	AppendTextRun(p, text, rPr)
}

// SetCellLines replaces a cell's content with a single paragraph whose lines
// are separated by soft line breaks (w:br), preserving properties.
func SetCellLines(table *etree.Element, row, col int, lines []string) {
	cell := cellAt(table, row, col)
	if cell == nil {
		return
	}
	rPr := FirstRunProperties(cell)
	pPr := FirstParagraphProperties(cell)
	clearChildren(cell, "tcPr")
	p := WordElement("p")
	if pPr != nil {
		p.AddChild(pPr)
	}
	cell.AddChild(p)
	for i, line := range lines {
		if i > 0 {
			AppendLineBreak(p, rPr)
		}
		AppendTextRun(p, line, rPr)
	}
}

// SetCellLimitedText fills a cell with text wrapped to the given line limit.
func SetCellLimitedText(table *etree.Element, row, col int, text string, limit LineLimit) {
	SetCellLines(table, row, col, WrapTextToLines(text, limit))
}

// SetCellLabelAndLimitedLines writes a label paragraph followed by one paragraph
// per wrapped line (matching the supervisor "听课笔记" appendix layout).
func SetCellLabelAndLimitedLines(table *etree.Element, row, col int, label, text string, limit LineLimit) {
	cell := cellAt(table, row, col)
	if cell == nil {
		return
	}
	rPr := FirstRunProperties(cell)
	pPr := FirstParagraphProperties(cell)
	clearChildren(cell, "tcPr")

	labelPara := WordElement("p")
	if pPr != nil {
		labelPara.AddChild(pPr.Copy())
	}
	cell.AddChild(labelPara)
	AppendTextRun(labelPara, label, rPr)

	widths := limitWidths(limit)
	lines := WrapTextToLines(text, limit)
	for len(lines) < len(widths) {
		lines = append(lines, "")
	}
	for _, line := range lines[:min(len(lines), len(widths))] {
		p := WordElement("p")
		if pPr != nil {
			p.AddChild(pPr.Copy())
		}
		cell.AddChild(p)
		AppendTextRun(p, line, rPr)
	}
}
