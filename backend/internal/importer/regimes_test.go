package importer

import (
	"encoding/base64"
	"strings"
	"testing"

	"ocm-backend/internal/xlsx"
)

// 长度预检按 schedule_regimes VARCHAR(32) 逐组拒绝超长作息制度名。
func TestParseRegimesOverlongNameRejected(t *testing.T) {
	headers := []string{"regime_name", "effective_month", "effective_day", "period_index", "start_time", "end_time"}
	rows := [][]any{
		{strings.Repeat("夏", 33), 5, 1, 1, "08:00", "09:00"},
		{strings.Repeat("夏", 33), 5, 1, 2, "09:00", "10:00"},
	}
	b, err := xlsx.BuildBytes("regimes", headers, rows)
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	clean, errs, groupCount, err := parseRegimes(base64.StdEncoding.EncodeToString(b))
	if err != nil {
		t.Fatalf("parseRegimes: %v", err)
	}
	if groupCount != 1 || len(clean) != 0 || len(errs) != 1 {
		t.Fatalf("超长制度名应整组拒绝：clean=%d errs=%v groupCount=%d", len(clean), errs, groupCount)
	}
}

// 严格数值解析：effective_month 非法的行逐行拒绝（消息带原文），其节次不入组。
func TestParseRegimesStrictNumerics(t *testing.T) {
	headers := []string{"regime_name", "effective_month", "effective_day", "period_index", "start_time", "end_time"}
	rows := [][]any{
		{"Standard", "五月", 1, 1, "08:00", "09:00"},
		{"Standard", 5, 1, 2, "09:00", "10:00"},
	}
	b, err := xlsx.BuildBytes("regimes", headers, rows)
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	clean, errs, groupCount, err := parseRegimes(base64.StdEncoding.EncodeToString(b))
	if err != nil {
		t.Fatalf("parseRegimes: %v", err)
	}
	if groupCount != 1 || len(clean) != 1 || len(errs) != 1 {
		t.Fatalf("非法行拒绝、合法行入组：clean=%d errs=%v groupCount=%d", len(clean), errs, groupCount)
	}
	if !strings.Contains(errs[0].Error, "五月") {
		t.Fatalf("错误消息应带原文 五月：%v", errs)
	}
}
