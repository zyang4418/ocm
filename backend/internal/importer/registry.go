package importer

import "context"

// Importer parses, validates, and writes one entity type from an uploaded xlsx
// payload. Analyze is the dry-run that produces a preview; Commit persists in a
// transaction. Both re-validate from the raw payload so database state changes
// between preview and commit are caught rather than trusting stale preview rows.
type Importer interface {
	// Analyze parses and validates payload without writing, returning the rows
	// that would be committed alongside per-row errors.
	Analyze(ctx context.Context, payload string) (Result, error)

	// Commit parses, validates, and inserts/upserts the valid rows in a single
	// transaction, returning the final per-row outcome.
	Commit(ctx context.Context, payload string) (Result, error)
}

// Registry maps a job type string to its Importer and the permission required
// to run it. The upload handler looks the type up here and enforces the
// permission before creating a job, so each entity's import is gated by the
// same permission as its manual manage action.
type Registry struct {
	importers  map[string]Importer
	permission map[string]string
}

func NewRegistry() *Registry {
	return &Registry{importers: map[string]Importer{}, permission: map[string]string{}}
}

// Register associates typ with imp and the authz permission needed to import it.
func (r *Registry) Register(typ, perm string, imp Importer) {
	r.importers[typ] = imp
	r.permission[typ] = perm
}

// Lookup returns the importer and required permission for typ. ok is false for
// an unknown type, which the upload handler reports as 404.
func (r *Registry) Lookup(typ string) (imp Importer, perm string, ok bool) {
	imp, ok = r.importers[typ]
	if !ok {
		return nil, "", false
	}
	return imp, r.permission[typ], true
}
