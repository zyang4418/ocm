package importer

import (
	"encoding/base64"
	"strings"
	"testing"

	"ocm-backend/internal/xlsx"
)

func offeringsPayload(t *testing.T, rows [][]any) string {
	t.Helper()
	headers := []string{
		"course", "code", "teaching_class", "semester", "teacher",
		"course_seq", "teacher_id", "teacher_title", "college",
		"max_students", "requirement", "weekly_hours", "note",
	}
	b, err := xlsx.BuildBytes("offerings", headers, rows)
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// 长度预检按 course_offerings 列宽（teacher 64 / requirement 16 等）逐行拒绝；
// 合法行不受影响。
func TestParseOfferingsOverlongFieldsRejected(t *testing.T) {
	catalog := map[string]int64{"TST101": 1}
	teaching := map[string]int64{"Class A-241": 2}
	payload := offeringsPayload(t, [][]any{
		{"Sample Course", "TST101", "Class A-241", "2026-2027-1", "Teacher One", "", "", "", "", 0, "必修课", 0, ""},
		{"Sample Course", "TST101", "Class A-241", "2026-2027-1", strings.Repeat("师", 65), "", "", "", "", 0, "", 0, ""},
		{"Sample Course", "TST101", "Class A-241", "2026-2027-1", "Teacher Two", "", "", "", "", 0, strings.Repeat("必", 17), 0, ""},
	})
	clean, errs, dataRows, err := parseOfferings(catalog, teaching, payload)
	if err != nil {
		t.Fatalf("parseOfferings: %v", err)
	}
	if dataRows != 3 || len(clean) != 1 || len(errs) != 2 {
		t.Fatalf("1 行通过、2 行拒绝：clean=%d errs=%v dataRows=%d", len(clean), errs, dataRows)
	}
}
