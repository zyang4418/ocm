package httpx

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PageParams carries the parsed page/page_size query parameters of a list
// request. List endpoints slice their result set with these values and answer
// with a Paged envelope (see RespondPaged).
type PageParams struct {
	Page     int
	PageSize int
}

const (
	// DefaultPageSize is the page size used when page_size is absent or invalid.
	DefaultPageSize = 100
	// MaxPageSize is the upper bound of page_size (clamped, never rejected).
	MaxPageSize = 500
)

// ParsePageParams reads page and page_size from the query with defaults and
// clamping: page defaults to 1 and is floored at 1 (no ceiling); page_size
// defaults to DefaultPageSize and is clamped to [1, MaxPageSize]. Non-numeric
// values fall back to the defaults.
func ParsePageParams(q url.Values) PageParams {
	return PageParams{
		Page:     parsePage(q.Get("page")),
		PageSize: parsePageSize(q.Get("page_size")),
	}
}

// parsePage returns the 1-based page number, floored at 1. Non-numeric values
// fall back to 1.
func parsePage(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// parsePageSize returns the page size with DefaultPageSize default, clamped to
// [1, MaxPageSize]. Non-numeric values fall back to the default.
func parsePageSize(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return n
}

// Offset returns the row offset of the page: (page-1) * pageSize.
func (p PageParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// ParseSearch returns the trimmed q parameter. An empty result means "no
// search filter"; handlers pass it straight to the store's Page* method.
func ParseSearch(q url.Values) string {
	return strings.TrimSpace(q.Get("q"))
}

// Paged is the envelope every list endpoint returns:
//
//	{"items": [...], "total": N, "page": P, "pageSize": S}
//
// total is the full result count across all pages (not the page length).
type Paged struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// RespondPaged writes the Paged envelope with status 200.
func RespondPaged(w http.ResponseWriter, items any, total int64, p PageParams) {
	RespondJSON(w, http.StatusOK, Paged{Items: items, Total: total, Page: p.Page, PageSize: p.PageSize})
}
