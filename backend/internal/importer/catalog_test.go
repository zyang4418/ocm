package importer

import (
	"encoding/base64"
	"strings"
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

// 缺整列 code 的文件在表头闸门报一条错误，而不是 N 行「code is required」。
func TestParseCatalogMissingCodeColumn(t *testing.T) {
	b, err := xlsx.BuildBytes("catalog", []string{"name", "credits"}, [][]any{
		{"Sample Course", 3.0},
	})
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	_, errs, _, err := parseCatalog(base64.StdEncoding.EncodeToString(b))
	if err == nil || len(errs) != 1 || !strings.Contains(errs[0].Error, "code") {
		t.Fatalf("缺 code 列应报表头错误：err=%v errs=%v", err, errs)
	}
}

// 长度预检按 course_catalog 列宽（name 128 / code 64）逐行拒绝，
// 而不是让 MySQL 1406 在 commit 时整事务回滚。
func TestParseCatalogOverlongFieldsRejected(t *testing.T) {
	payload := catalogPayload(t, [][]any{
		{strings.Repeat("课", 129), "TST201", 3.0, 48, "", "", ""},
		{"Sample Course", strings.Repeat("X", 65), 3.0, 48, "", "", ""},
	})
	clean, errs, _, err := parseCatalog(payload)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if len(clean) != 0 || len(errs) != 2 {
		t.Fatalf("超长字段应逐行拒绝：clean=%d errs=%v", len(clean), errs)
	}
}

// 严格数值解析：空值回退默认（列可选），非法值逐行拒绝且消息带原文；
// upsert 键 code 在文件内重复时报行错误（对齐 offerings），不再静默 last-wins。
func TestParseCatalogStrictNumericsAndDuplicateCode(t *testing.T) {
	payload := catalogPayload(t, [][]any{
		{"Sample Course A", "TST301", "", "", "", "", ""},     // 数值空 → 默认 0，通过
		{"Sample Course B", "TST302", "3学分", 48, "", "", ""},  // credits 非法
		{"Sample Course C", "TST303", 3.0, "4-8", "", "", ""}, // total_hours 非法
		{"Sample Course D", "TST304", 3.5, 48, "", "", ""},    // 合法小数
		{"Sample Course E", "TST304", 2.0, 32, "", "", ""},    // 与 D 同码 → 重复
	})
	clean, errs, _, err := parseCatalog(payload)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if len(clean) != 2 || len(errs) != 3 {
		t.Fatalf("2 行通过、3 行拒绝：clean=%d errs=%v", len(clean), errs)
	}
	if clean[0].Credits != 0 || clean[0].TotalHours != 0 {
		t.Fatalf("空数值列应回退 0：credits=%v totalHours=%v", clean[0].Credits, clean[0].TotalHours)
	}
	if clean[1].Credits != 3.5 {
		t.Fatalf("合法小数应原样解析：credits=%v", clean[1].Credits)
	}
	joined := errs[0].Error + errs[1].Error + errs[2].Error
	for _, want := range []string{"3学分", "4-8", "重复课程代码"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("错误消息应包含 %q：%v", want, errs)
		}
	}
}
