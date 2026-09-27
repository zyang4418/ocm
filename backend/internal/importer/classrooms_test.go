package importer

import (
	"encoding/base64"
	"strings"
	"testing"

	"ocm-backend/internal/xlsx"
)

func classroomsPayload(t *testing.T, rows [][]any) string {
	t.Helper()
	headers := []string{"name", "building", "capacity", "type", "floor", "campus", "status", "description"}
	b, err := xlsx.BuildBytes("classrooms", headers, rows)
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// 严格数值解析：capacity 非法值（如「80座」）逐行拒绝且消息带原文，
// 不再被 atoiOr 静默归 0 后误报「capacity must be greater than 0」。
func TestParseClassroomsStrictCapacity(t *testing.T) {
	payload := classroomsPayload(t, [][]any{
		{"Room-101", "Building A", "80座", "", "", "", "", ""},
		{"Room-102", "Building A", "60", "", "", "", "", ""},
		{"Room-103", "Building A", "80", "", "", "", "", ""},
	})
	clean, errs, _, err := parseClassrooms(payload)
	if err != nil {
		t.Fatalf("parseClassrooms: %v", err)
	}
	if len(clean) != 2 || len(errs) != 1 {
		t.Fatalf("2 行通过、1 行拒绝：clean=%d errs=%v", len(clean), errs)
	}
	if !strings.Contains(errs[0].Error, "80座") {
		t.Fatalf("错误消息应带原文 80座：%v", errs)
	}
}

// Upsert 键是 name：文件内两行同名报行错误，不静默 last-wins。
func TestParseClassroomsDuplicateNameRejected(t *testing.T) {
	payload := classroomsPayload(t, [][]any{
		{"Room-201", "Building B", "60", "", "", "", "", ""},
		{"Room-201", "Building C", "100", "", "", "", "", ""},
	})
	clean, errs, _, err := parseClassrooms(payload)
	if err != nil {
		t.Fatalf("parseClassrooms: %v", err)
	}
	if len(clean) != 1 || len(errs) != 1 || !strings.Contains(errs[0].Error, "重复教室") {
		t.Fatalf("同名两行应报重复：clean=%d errs=%v", len(clean), errs)
	}
}
