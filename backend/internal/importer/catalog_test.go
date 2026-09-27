package importer

import (
	"encoding/base64"
	"testing"

	"ocm-backend/internal/xlsx"
)

func catalogPayload(t *testing.T, rows [][]any) string {
	t.Helper()
	headers := []string{"name", "code", "credits", "total_hours", "category", "exam_type", "description"}
	b, err := xlsx.BuildBytes("catalog", headers, rows)
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// 课程库身份键是 code：同名不同码的两门课必须各自成行（回归：旧模型按 name
// upsert 会把后者静默覆盖进前者）；缺 code 的行被逐行拒绝（code 必填）。
func TestParseCatalogSameNameDifferentCodes(t *testing.T) {
	payload := catalogPayload(t, [][]any{
		{"Sample Course", "TST101", 3.0, 48, "", "考试", ""},
		{"Sample Course", "TST102", 2.0, 32, "", "考查", ""},
	})
	clean, errs, dataRows, err := parseCatalog(payload)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if dataRows != 2 || len(clean) != 2 || len(errs) != 0 {
		t.Fatalf("同名不同码应各自成行：clean=%d errs=%v dataRows=%d", len(clean), errs, dataRows)
	}
	if clean[0].Code == clean[1].Code {
		t.Fatalf("两行 code 不应相同：%q", clean[0].Code)
	}
}

func TestParseCatalogMissingCodeRejected(t *testing.T) {
	payload := catalogPayload(t, [][]any{
		{"Sample Course", "", 3.0, 48, "", "考试", ""},
	})
	clean, errs, _, err := parseCatalog(payload)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if len(clean) != 0 || len(errs) != 1 {
		t.Fatalf("缺 code 应逐行拒绝：clean=%d errs=%v", len(clean), errs)
	}
}
