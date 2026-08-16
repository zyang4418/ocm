package docx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// buildTestDocx returns a minimal .docx with a known document.xml body: one
// paragraph with two underlined runs, and a single-cell table.
func buildTestDocx(t *testing.T) []byte {
	t.Helper()
	const documentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:p><w:r><w:rPr><w:u/></w:rPr><w:t>课程名称：</w:t></w:r>` +
		`<w:r><w:rPr><w:u/></w:rPr><w:t>________</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:tcPr/><w:p><w:r><w:t>旧内容</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte(documentXML))
	_ = zw.Close()
	return buf.Bytes()
}

func readDocumentXML(t *testing.T, docx []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("open rendered docx: %v", err)
	}
	b, err := readPart(zr, documentPart)
	if err != nil {
		t.Fatalf("read document.xml: %v", err)
	}
	return string(b)
}

func TestRenderFillsUnderlinedSlotsAndCell(t *testing.T) {
	tmpl := buildTestDocx(t)

	out, err := Render(tmpl, func(d *Document) error {
		if err := FillUnderlinedSlots(d, "课程名称：", []string{"高等数学"}); err != nil {
			return err
		}
		SetCellText(d.Tables()[0], 0, 0, "新内容")
		return nil
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	xml := readDocumentXML(t, out)
	if !strings.Contains(xml, "高等数学") {
		t.Errorf("underlined slot not filled: %s", xml)
	}
	if !strings.Contains(xml, "新内容") {
		t.Errorf("table cell not filled: %s", xml)
	}
	if strings.Contains(xml, "旧内容") {
		t.Errorf("old cell text not cleared: %s", xml)
	}
	// The xml declaration must survive the round-trip.
	if !strings.Contains(xml, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`) {
		t.Errorf("xml declaration lost: %s", xml)
	}
}

func TestWrapTextToLines(t *testing.T) {
	got := WrapTextToLines("abcdef", LineLimit{MaxLines: 2, MaxCharsPerLine: 3})
	want := []string{"abc", "def"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	// Per-line widths.
	got = WrapTextToLines("hello", LineLimit{LineCharLimits: []int{2, 1, 1}})
	if strings.Join(got, "|") != "he|l|l" {
		t.Fatalf("per-line widths wrong: %v", got)
	}

	// Explicit newline starts a fresh line.
	got = WrapTextToLines("ab\ncd", LineLimit{MaxLines: 4, MaxCharsPerLine: 3})
	if strings.Join(got, "|") != "ab|cd" {
		t.Fatalf("newline handling wrong: %v", got)
	}

	// Excess beyond the line count is dropped.
	got = WrapTextToLines("abcdef", LineLimit{MaxLines: 1, MaxCharsPerLine: 3})
	if strings.Join(got, "|") != "abc" {
		t.Fatalf("overflow not dropped: %v", got)
	}
}

func TestMarkers(t *testing.T) {
	if Check(true) != "√" || Check(false) != "" {
		t.Fatalf("Check markers wrong")
	}
	if Checkbox(true) != "☑" || Checkbox(false) != "☐" {
		t.Fatalf("Checkbox markers wrong")
	}
}
