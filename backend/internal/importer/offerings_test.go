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

// 严格数值解析：max_students / weekly_hours 非法值逐行拒绝且消息带原文。
func TestParseOfferingsStrictNumerics(t *testing.T) {
	catalog := map[string]int64{"TST101": 1}
	teaching := map[string]int64{"Class A-241": 2}
	payload := offeringsPayload(t, [][]any{
		{"Sample Course", "TST101", "Class A-241", "2026-2027-1", "Teacher One", "", "", "", "", "50人", "", "", ""},
		{"Sample Course", "TST101", "Class A-241", "2026-2027-1", "Teacher Two", "", "", "", "", "", "", "4.5学时", ""},
	})
	clean, errs, _, err := parseOfferings(catalog, teaching, payload)
	if err != nil {
		t.Fatalf("parseOfferings: %v", err)
	}
	if len(clean) != 0 || len(errs) != 2 {
		t.Fatalf("2 行应全部拒绝：clean=%d errs=%v", len(clean), errs)
	}
	joined := errs[0].Error + errs[1].Error
	for _, want := range []string{"50人", "4.5学时"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("错误消息应带原文 %q：%v", want, errs)
		}
	}
}
