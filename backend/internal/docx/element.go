package docx

import (
	"strings"

	"github.com/beevik/etree"
)

// WordElement creates a new element in the WordprocessingML namespace (w:local).
func WordElement(local string) *etree.Element {
	return etree.NewElement("w:" + local)
}

// IsWord reports whether e is a WordprocessingML element with the given local
// name. Word's document.xml binds the "w" prefix consistently, so checking the
// prefix (rather than the local name alone) avoids matching same-named elements
// from other namespaces such as DrawingML's "a:p".
func IsWord(e *etree.Element, local string) bool {
	return e != nil && e.Space == "w" && e.Tag == local
}

// Children returns the direct child elements of e with the given local name,
// preserving order.
func Children(e *etree.Element, local string) []*etree.Element {
	var out []*etree.Element
	for _, t := range e.Child {
		if c, ok := t.(*etree.Element); ok && IsWord(c, local) {
			out = append(out, c)
		}
	}
	return out
}

// ChildWord returns the first direct child element of e with the given local
// name, or nil.
func ChildWord(e *etree.Element, local string) *etree.Element {
	for _, c := range Children(e, local) {
		return c
	}
	return nil
}

// Descendants returns every descendant element of e with the given local name,
// in depth-first document order. e itself is not included.
func Descendants(e *etree.Element, local string) []*etree.Element {
	var out []*etree.Element
	walkDescendants(e, local, &out)
	return out
}

func walkDescendants(e *etree.Element, local string, out *[]*etree.Element) {
	for _, t := range e.Child {
		c, ok := t.(*etree.Element)
		if !ok {
			continue
		}
		if IsWord(c, local) {
			*out = append(*out, c)
		}
		walkDescendants(c, local, out)
	}
}

// NodeText returns the concatenated text of all w:t descendants, in order.
func NodeText(e *etree.Element) string {
	var sb strings.Builder
	for _, t := range Descendants(e, "t") {
		sb.WriteString(t.Text())
	}
	return sb.String()
}

// FirstRunProperties returns a deep copy of the first w:rPr found in e. For a
// paragraph it searches direct child runs; for any other element (e.g. a cell)
// it searches all descendant runs. It returns nil when no run properties exist.
func FirstRunProperties(e *etree.Element) *etree.Element {
	var runs []*etree.Element
	if IsWord(e, "p") {
		runs = Children(e, "r")
	} else {
		runs = Descendants(e, "r")
	}
	for _, r := range runs {
		if rPr := ChildWord(r, "rPr"); rPr != nil {
			return rPr.Copy()
		}
	}
	return nil
}

// FirstParagraphProperties returns a deep copy of the first w:pPr of the cell's
// first direct child paragraph, or nil.
func FirstParagraphProperties(cell *etree.Element) *etree.Element {
	p := ChildWord(cell, "p")
	if p == nil {
		return nil
	}
	if pPr := ChildWord(p, "pPr"); pPr != nil {
		return pPr.Copy()
	}
	return nil
}

// clearChildren keeps only the direct child elements whose local name equals
// keep, dropping every other token (runs, breaks, stray character data).
func clearChildren(e *etree.Element, keep string) {
	var kept []etree.Token
	for _, t := range e.Child {
		if c, ok := t.(*etree.Element); ok && IsWord(c, keep) {
			kept = append(kept, t)
		}
	}
	e.Child = kept
	e.ReindexChildren()
}

// ClearParagraph removes every child of the paragraph except its w:pPr.
func ClearParagraph(p *etree.Element) {
	clearChildren(p, "pPr")
}

// setSpacePreserve sets or clears xml:space="preserve" on a w:t element so
// leading/trailing/consecutive spaces survive Word's whitespace normalization.
func setSpacePreserve(t *etree.Element, text string) {
	needs := strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") || strings.Contains(text, "  ")
	if needs {
		t.CreateAttr("xml:space", "preserve")
	} else {
		t.RemoveAttr("xml:space")
	}
}

// AppendTextRun appends a w:r carrying text to the paragraph, reusing rPr when
// non-nil so the run inherits the template's original font/size.
func AppendTextRun(p *etree.Element, text string, rPr *etree.Element) {
	r := WordElement("r")
	if rPr != nil {
		r.AddChild(rPr.Copy())
	}
	t := WordElement("t")
	setSpacePreserve(t, text)
	t.SetText(text)
	r.AddChild(t)
	p.AddChild(r)
}

// AppendLineBreak appends a w:r containing a w:br (soft line break).
func AppendLineBreak(p *etree.Element, rPr *etree.Element) {
	r := WordElement("r")
	if rPr != nil {
		r.AddChild(rPr.Copy())
	}
	r.AddChild(WordElement("br"))
	p.AddChild(r)
}

// SetParagraphText replaces the paragraph's content with a single run carrying
// text, preserving the paragraph's first run properties.
func SetParagraphText(p *etree.Element, text string) {
	rPr := FirstRunProperties(p)
	ClearParagraph(p)
	AppendTextRun(p, text, rPr)
}

// RunText returns the concatenated text of the run's direct w:t children.
func RunText(r *etree.Element) string {
	var sb strings.Builder
	for _, t := range Children(r, "t") {
		sb.WriteString(t.Text())
	}
	return sb.String()
}

// SetRunText replaces the run's text with the given value, keeping only the
// first w:t element and managing xml:space.
func SetRunText(r *etree.Element, text string) {
	texts := Children(r, "t")
	if len(texts) == 0 {
		t := WordElement("t")
		setSpacePreserve(t, text)
		t.SetText(text)
		r.AddChild(t)
		return
	}
	for _, extra := range texts[1:] {
		r.RemoveChild(extra)
	}
	first := texts[0]
	setSpacePreserve(first, text)
	first.SetText(text)
}

// IsUnderlinedRun reports whether the run's properties contain a w:u element.
func IsUnderlinedRun(r *etree.Element) bool {
	rPr := ChildWord(r, "rPr")
	return rPr != nil && ChildWord(rPr, "u") != nil
}
