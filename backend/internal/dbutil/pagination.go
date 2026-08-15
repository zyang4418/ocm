package dbutil

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Pagination carries LIMIT/OFFSET for a sliced list query. The zero value
// means "no limit": internal callers (importer loadRefs, Timetable, export
// handlers, validation paths) pass the zero value to keep receiving the full
// result set, identical to the unpaginated List* methods.
type Pagination struct {
	Limit  int
	Offset int
}

// Paged reports whether a LIMIT is set.
func (p Pagination) Paged() bool { return p.Limit > 0 }

// AppendLimit appends ` LIMIT ? OFFSET ?` (and the two args) when p.Paged();
// otherwise it returns q and args unchanged. Always call it after the ORDER BY
// clause — LIMIT must come last in a SELECT.
func (p Pagination) AppendLimit(q string, args []any) (string, []any) {
	if !p.Paged() {
		return q, args
	}
	return q + ` LIMIT ? OFFSET ?`, append(args, p.Limit, p.Offset)
}

// EscapeLike escapes LIKE wildcards so user search input is matched literally:
// \ -> \\, % -> \%, _ -> \_. Callers must also escape the pattern and keep
// MySQL's default ESCAPE ('\').
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// LikePattern wraps an already-escaped term in '%' for a contains-match, e.g.
// `name LIKE ?` with arg LikePattern(EscapeLike(q)).
func LikePattern(s string) string {
	return "%" + s + "%"
}

// CountRows runs `SELECT COUNT(*) <fromWhere>` with args. fromWhere is the
// JOIN + WHERE fragment shared with the page query (e.g. a sessionJoin +
// WHERE 1=1 ... fragment) and must not contain LIMIT/OFFSET or an ORDER BY.
func CountRows(ctx context.Context, db *sql.DB, fromWhere string, args []any) (int64, error) {
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) `+fromWhere, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	return total, nil
}
