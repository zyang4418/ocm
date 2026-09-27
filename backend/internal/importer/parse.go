package importer

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

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

// parseIntCol parses an optional int column: empty returns def (the column is
// optional), a non-empty value must parse or the caller rejects the row with
// the message. Messages carry the raw cell so hand-edited values like "80座"
// surface verbatim instead of silently becoming the default (atoiOr behavior).
// For required int columns use parseIntField.
func parseIntCol(s, name string, def int) (int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, ""
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Sprintf("%s 非法：%q（须为整数）", name, s)
	}
	return n, ""
}

// parseFloatCol parses an optional float column: empty returns def, a non-empty
// value must parse or the caller rejects the row with the message.
func parseFloatCol(s, name string, def float64) (float64, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, ""
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Sprintf("%s 非法：%q（须为数字）", name, s)
	}
	return n, ""
}
