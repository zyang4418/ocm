package importer

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"ocm-backend/internal/xlsx"
)

// parseWorkbook base64-decodes the stored payload (the uploaded xlsx bytes) and
// reads the first worksheet into header-mapped rows. Payload is stored base64-
// encoded because import_jobs.payload is a TEXT column and an xlsx file is a
// binary zip; storing the raw bytes there would corrupt on the UTF-8 round-trip.
func parseWorkbook(payload string) ([]string, []map[string]string, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("解码上传内容失败：%w", err)
	}
	return xlsx.MapRows(raw)
}

// requireColumns reports an error if any of cols is absent from headers. The
// returned RowError carries the user-facing Chinese message; the error is non-nil
// so the caller can abort the parse (a missing required column makes every row
// unresolvable).
func requireColumns(headers []string, cols ...string) (RowError, bool) {
	for _, c := range cols {
		if !xlsx.Has(headers, c) {
			return RowError{Row: 1, Error: "表头缺少必需列：" + c}, false
		}
	}
	return RowError{}, true
}

// atoiOr parses s as an int, returning def when s is empty or not a valid
// integer. Importers use it to read numeric columns that may be blank.
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
