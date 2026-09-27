package importer

import (
	"encoding/base64"
	"strings"
	"testing"

	"ocm-backend/internal/xlsx"
)

func adminClassesPayload(t *testing.T, rows [][]any) string {
	t.Helper()
	headers := []string{"grade", "name", "note"}
	b, err := xlsx.BuildBytes("admin_classes", headers, rows)
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// grade 是 UNIQUE(grade, name) 的一半、教学班导入按 grade|name 匹配成员的键：
// 必填且必须是 4 位入学年份，空值或怪格式逐行拒绝（回归：旧校验只查 name，
// 空 grade 入库后教学班导入静默失配）。
func TestParseAdminClassesGradeRequired(t *testing.T) {
	payload := adminClassesPayload(t, [][]any{
		{"2024", "Class A-241", ""},
		{"", "Class A-242", ""},
		{"24级", "Class A-243", ""},
		{"20241", "Class A-244", ""},
	})
	clean, errs, _, err := parseAdminClasses(payload)
	if err != nil {
		t.Fatalf("parseAdminClasses: %v", err)
	}
	if len(clean) != 1 || len(errs) != 3 {
		t.Fatalf("1 行通过、3 行拒绝：clean=%d errs=%v", len(clean), errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Error, "grade") {
			t.Fatalf("错误应指向 grade 列：%q", e.Error)
		}
	}
}

// 缺 grade 列的文件在表头闸门就被拦截，而不是 N 行「grade is required」。
func TestParseAdminClassesMissingGradeColumn(t *testing.T) {
	b, err := xlsx.BuildBytes("admin_classes", []string{"name", "note"}, [][]any{
		{"Class A-241", ""},
	})
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	_, errs, _, err := parseAdminClasses(base64.StdEncoding.EncodeToString(b))
	if err == nil || len(errs) != 1 || !strings.Contains(errs[0].Error, "grade") {
		t.Fatalf("缺 grade 列应报表头错误：err=%v errs=%v", err, errs)
	}
}

// 长度预检把 MySQL 1406（Data too long，整事务回滚）降级为逐行拒绝。
func TestParseAdminClassesOverlongNameRejected(t *testing.T) {
	payload := adminClassesPayload(t, [][]any{
		{"2024", strings.Repeat("班", 65), ""},
	})
	clean, errs, _, err := parseAdminClasses(payload)
	if err != nil {
		t.Fatalf("parseAdminClasses: %v", err)
	}
	if len(clean) != 0 || len(errs) != 1 || !strings.Contains(errs[0].Error, "64") {
		t.Fatalf("超长 name 应逐行拒绝：clean=%d errs=%v", len(clean), errs)
	}
}

// Upsert 键是 (grade, name)：文件内两行同键报行错误，不静默 last-wins。
func TestParseAdminClassesDuplicateKeyRejected(t *testing.T) {
	payload := adminClassesPayload(t, [][]any{
		{"2024", "Class A-241", "note one"},
		{"2024", "Class A-241", "note two"},
	})
	clean, errs, _, err := parseAdminClasses(payload)
	if err != nil {
		t.Fatalf("parseAdminClasses: %v", err)
	}
	if len(clean) != 1 || len(errs) != 1 || !strings.Contains(errs[0].Error, "重复行政班") {
		t.Fatalf("同键两行应报重复：clean=%d errs=%v", len(clean), errs)
	}
}
