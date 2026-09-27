package dbutil

import (
	"fmt"
	"unicode/utf8"
)

// MaxRunes reports a user-facing error message when value is longer than max
// runes. MySQL VARCHAR(n) under utf8mb4 counts characters, not bytes, so rune
// count is what the column actually accepts. Normalizers run it per string
// column so an overlong value — most often a pasted or hand-edited xlsx cell —
// is rejected per row at parse time instead of surfacing as a MySQL 1406
// (Data too long) that aborts the importer's whole commit transaction.
func MaxRunes(field, value string, max int) (string, bool) {
	if utf8.RuneCountInString(value) > max {
		return fmt.Sprintf("%s must be at most %d characters", field, max), false
	}
	return "", true
}
