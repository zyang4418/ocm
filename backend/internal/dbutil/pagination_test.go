package dbutil

import (
	"reflect"
	"testing"
)

func TestAppendLimit(t *testing.T) {
	// Zero value must return the input unchanged — this is the guarantee that
	// internal callers (importers, exports) keep getting full-range results.
	q, args := Pagination{}.AppendLimit(`SELECT 1 ORDER BY id`, []any{})
	if q != `SELECT 1 ORDER BY id` || len(args) != 0 {
		t.Errorf("zero Pagination changed query: %q args=%v", q, args)
	}

	p := Pagination{Limit: 100, Offset: 200}
	q, args = p.AppendLimit(`SELECT 1 ORDER BY id`, []any{"x"})
	if q != `SELECT 1 ORDER BY id LIMIT ? OFFSET ?` {
		t.Errorf("paged query = %q", q)
	}
	if !reflect.DeepEqual(args, []any{"x", 100, 200}) {
		t.Errorf("paged args = %v, want [x 100 200]", args)
	}
}

func TestEscapeLike(t *testing.T) {
	cases := []struct{ in, want string }{
		{"50%", `50\%`},
		{"a_b", `a\_b`},
		{`a\b`, `a\\b`},
		{`\%_`, `\\\%\_`},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := EscapeLike(c.in); got != c.want {
			t.Errorf("EscapeLike(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLikePattern(t *testing.T) {
	if got := LikePattern(EscapeLike("50%")); got != `%50\%%` {
		t.Errorf("LikePattern = %q, want %%50\\%%%%", got)
	}
	if got := LikePattern("数学"); got != "%数学%" {
		t.Errorf("LikePattern = %q, want %%数学%%", got)
	}
}
