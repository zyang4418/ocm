package brand

import "testing"

func TestNameDefaults(t *testing.T) {
	t.Setenv("BRAND_NAME", "")
	if got := Name(); got != DefaultName {
		t.Fatalf("empty BRAND_NAME must fall back to %q, got %q", DefaultName, got)
	}
}

func TestNameFromEnv(t *testing.T) {
	t.Setenv("BRAND_NAME", "示例大学智慧教室")
	if got := Name(); got != "示例大学智慧教室" {
		t.Fatalf("BRAND_NAME must be honored, got %q", got)
	}
}

func TestNameTrimsBlank(t *testing.T) {
	t.Setenv("BRAND_NAME", "   ")
	if got := Name(); got != DefaultName {
		t.Fatalf("blank BRAND_NAME must fall back to %q, got %q", DefaultName, got)
	}
}
