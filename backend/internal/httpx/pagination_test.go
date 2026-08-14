package httpx

import (
	"net/url"
	"testing"
)

func TestParsePageParams(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantPage int
		wantSize int
	}{
		{"defaults", "", 1, DefaultPageSize},
		{"explicit", "page=3&page_size=200", 3, 200},
		{"clamp size high", "page_size=1000", 1, MaxPageSize},
		{"clamp size low", "page_size=0", 1, 100},
		{"clamp page low", "page=0", 1, 100},
		{"clamp page negative", "page=-3", 1, 100},
		{"non-numeric page", "page=abc", 1, 100},
		{"non-numeric size", "page_size=xyz", 1, 100},
		{"empty values", "page=&page_size=", 1, 100},
	}
	for _, c := range cases {
		p := ParsePageParams(url.Values{})
		if c.query != "" {
			p = ParsePageParams(mustQuery(t, c.query))
		}
		if p.Page != c.wantPage || p.PageSize != c.wantSize {
			t.Errorf("%s: ParsePageParams(%q) = (%d,%d), want (%d,%d)",
				c.name, c.query, p.Page, p.PageSize, c.wantPage, c.wantSize)
		}
	}
}

func TestPageParamsOffset(t *testing.T) {
	if got := (PageParams{Page: 1, PageSize: 100}).Offset(); got != 0 {
		t.Errorf("page 1 offset = %d, want 0", got)
	}
	if got := (PageParams{Page: 3, PageSize: 100}).Offset(); got != 200 {
		t.Errorf("page 3 offset = %d, want 200", got)
	}
	if got := (PageParams{Page: 2, PageSize: 500}).Offset(); got != 500 {
		t.Errorf("page 2 size 500 offset = %d, want 500", got)
	}
}

func TestParseSearch(t *testing.T) {
	if got := ParseSearch(url.Values{}); got != "" {
		t.Errorf("ParseSearch(empty) = %q, want \"\"", got)
	}
	if got := ParseSearch(url.Values{"q": {"  数学 "}}); got != "数学" {
		t.Errorf("ParseSearch = %q, want 数学", got)
	}
	if got := ParseSearch(url.Values{"q": {"   "}}); got != "" {
		t.Errorf("ParseSearch(blank) = %q, want \"\"", got)
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", raw, err)
	}
	return v
}
