package observation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Renderer is the pluggable document backend for observations. The open-source
// layer defines only this contract; a deployment's customization layer
// implements it with its own form templates, schemas and .docx fillers (built
// on the generic internal/docx engine). A nil Renderer leaves the module fully
// functional for CRUD/submit but disables the templates and export endpoints.
type Renderer interface {
	// Templates returns the form templates and schema served by the templates
	// endpoint. The value is JSON-encoded as-is.
	Templates() (any, error)
	// Validate checks obs against its template schema and returns the missing
	// required fields (empty means valid). Called before a submit is accepted.
	Validate(obs *Observation) ([]string, error)
	// Render writes the .docx document for obs to w.
	Render(ctx context.Context, obs *Observation, w io.Writer) error
}

// docxContentType is the MIME type of a Word document.
const docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// serveDocx writes buf as a .docx download with an RFC 6266 disposition. The
// filename is ASCII (id + template type), so a single filename attribute
// suffices; it still goes through url.QueryEscape for the filename* fallback so
// the helper stays reusable for non-ASCII names.
func serveDocx(w http.ResponseWriter, filename string, buf []byte) {
	w.Header().Set("Content-Type", docxContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.QueryEscape(filename)))
	_, _ = w.Write(buf)
}
