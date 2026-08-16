package observation

import (
	"strings"
	"testing"
)

// TestObsJoinLeadingWhitespace guards against the SQL string-concatenation bug
// where obsJoin glued onto the preceding column list (u.display_nameFROM ...),
// producing MySQL 1064 syntax errors in Get/Page. The ocm convention (see
// booking's bookingJoin and course's sessionJoin) is a leading newline in the
// JOIN fragment so it can never stick to the column before it.
func TestObsJoinLeadingWhitespace(t *testing.T) {
	if !strings.HasPrefix(obsJoin, "\n") && !strings.HasPrefix(obsJoin, " ") && !strings.HasPrefix(obsJoin, "\t") {
		t.Fatalf("obsJoin must start with whitespace so it cannot glue onto the preceding column")
	}

	full := `SELECT ` + obsColumns + `, c.name, c.code, ofr.teacher, tc.name, cr.name, u.display_name` + obsJoin
	if strings.Contains(full, "display_nameFROM") {
		t.Fatalf("obsJoin glued onto the preceding column: %q", full)
	}
}
