// Package docx provides a generic, template-agnostic engine for filling
// Microsoft Word .docx templates.
//
// A .docx file is a ZIP archive whose word/document.xml holds the document
// body. The engine parses that XML with beevik/etree (BSD-2-Clause, a Go
// analogue of Python's ElementTree), applies a caller-supplied filler to the
// element tree, and re-serializes it — mirroring how the legacy observation
// exporter drove lxml directly over word/document.xml.
//
// It intentionally exposes only low-level OOXML primitives (runs, paragraphs,
// table cells, underline slots, check markers, line wrapping). Template layout
// and business schema — which underline slot to fill with which value, which
// table row is a score, what the per-line character limits are — belong to the
// caller, so the engine stays reusable across any school- or domain-specific
// form.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/beevik/etree"
)

const (
	// WordNamespace is the WordprocessingML namespace URI. Word consistently
	// binds it to the "w" prefix inside word/document.xml.
	WordNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

	// XMLNamespace is the standard xml namespace (xml:space="preserve").
	XMLNamespace = "http://www.w3.org/XML/1998/namespace"
)

// documentPart is the ZIP entry that carries the document body.
const documentPart = "word/document.xml"

// Document wraps the parsed word/document.xml element tree.
type Document struct {
	root *etree.Element
}

// Root returns the w:document root element.
func (d *Document) Root() *etree.Element { return d.root }

// Paragraphs returns every w:p element in document order.
func (d *Document) Paragraphs() []*etree.Element {
	return Descendants(d.root, "p")
}

// Tables returns every w:tbl element in document order.
func (d *Document) Tables() []*etree.Element {
	return Descendants(d.root, "tbl")
}

// ParagraphByPrefix returns the first w:p whose text (after trimming leading
// whitespace) starts with prefix, or an error if none matches. It mirrors the
// legacy template's prefix-anchored slot filling.
func (d *Document) ParagraphByPrefix(prefix string) (*etree.Element, error) {
	for _, p := range d.Paragraphs() {
		if hasPrefix(NodeText(p), prefix) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("docx: template paragraph not found: %q", prefix)
}

// Render parses template (raw .docx bytes), applies filler to its document
// tree, and returns the rendered .docx bytes. Non-document ZIP entries are
// copied through unchanged so embedded media, styles and relationships survive
// a fill.
func Render(template []byte, filler func(*Document) error) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return nil, fmt.Errorf("docx: open template: %w", err)
	}

	documentXML, err := readPart(zr, documentPart)
	if err != nil {
		return nil, err
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(documentXML); err != nil {
		return nil, fmt.Errorf("docx: parse %s: %w", documentPart, err)
	}
	wrapped := &Document{root: doc.Root()}
	if wrapped.root == nil {
		return nil, fmt.Errorf("docx: %s has no root element", documentPart)
	}
	if err := filler(wrapped); err != nil {
		return nil, err
	}
	renderedXML, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("docx: serialize %s: %w", documentPart, err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		hdr := f.FileHeader
		dst, err := zw.CreateHeader(&hdr)
		if err != nil {
			return nil, fmt.Errorf("docx: create zip entry %s: %w", f.Name, err)
		}
		if f.Name == documentPart {
			if _, err := dst.Write(renderedXML); err != nil {
				return nil, fmt.Errorf("docx: write %s: %w", documentPart, err)
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("docx: open zip entry %s: %w", f.Name, err)
		}
		_, copyErr := io.Copy(dst, rc)
		closeErr := rc.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("docx: copy zip entry %s: %w", f.Name, copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("docx: close zip entry %s: %w", f.Name, closeErr)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("docx: close archive: %w", err)
	}
	return buf.Bytes(), nil
}

// readPart returns the bytes of the named ZIP entry.
func readPart(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("docx: open %s: %w", name, err)
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("docx: %s not found in template", name)
}

// hasPrefix reports whether the left-trimmed text starts with prefix. The
// non-breaking space (U+00A0) is treated as whitespace, matching how Word
// occasionally pads paragraph markers.
func hasPrefix(text, prefix string) bool {
	return strings.HasPrefix(strings.TrimLeft(text, " \t\r\n\u00a0"), prefix)
}
