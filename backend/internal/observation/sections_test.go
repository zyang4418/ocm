package observation

import "testing"

func TestNormalizeSections(t *testing.T) {
	got, err := NormalizeSections([]int{3, 1, 2, 1})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("expected sorted deduped [1 2 3], got %v", got)
	}

	if got, _ := NormalizeSections(nil); len(got) != 0 {
		t.Fatalf("nil sections should normalize to empty, got %v", got)
	}

	if _, err := NormalizeSections([]int{0}); err == nil {
		t.Fatalf("expected error for non-positive section")
	}
	if _, err := NormalizeSections([]int{-1}); err == nil {
		t.Fatalf("expected error for negative section")
	}
}

func TestBuildSectionsKey(t *testing.T) {
	// These digests are sha256 of the canonical JSON ([1,2,3], [1,2], []),
	// matching the legacy build_sections_key byte-for-byte.
	cases := map[string]string{
		"[1,2,3]": "a615eeaee21de5179de080de8c3052c8da901138406ba71c38c032845f7d54f4",
		"[1,2]":   "49a64717d5d4cb19952e6eac2946415cf6879adacf9908e7d872332d32c6e684",
		"[]":      "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945",
	}
	if got := BuildSectionsKey([]int{1, 2, 3}); got != cases["[1,2,3]"] {
		t.Fatalf("key [1,2,3]: got %s", got)
	}
	if got := BuildSectionsKey([]int{1, 2}); got != cases["[1,2]"] {
		t.Fatalf("key [1,2]: got %s", got)
	}
	if got := BuildSectionsKey([]int{}); got != cases["[]"] {
		t.Fatalf("key []: got %s", got)
	}

	// Input order must not affect the key (it is normalized first by the
	// caller; the key itself is over the canonical list).
	if a, b := BuildSectionsKey([]int{3, 2, 1}), BuildSectionsKey([]int{1, 2, 3}); a != b {
		t.Fatalf("key must be order-independent")
	}
}

func TestDeriveSummary(t *testing.T) {
	scores, totalScore, content, remark := deriveSummary([]byte(`{
		"indicatorScores": {"attitude": "excellent"},
		"totalScore": 88,
		"contentOutline": "提纲",
		"comments": {"advantages": "好", "other": "无"}
	}`))
	if scores != `{"attitude": "excellent"}` {
		t.Fatalf("scores: got %s", scores)
	}
	if totalScore == nil || totalScore.(float64) != 88 {
		t.Fatalf("totalScore: got %v", totalScore)
	}
	if content != "提纲" {
		t.Fatalf("content: got %q", content)
	}
	if remark != "无" {
		t.Fatalf("remark: got %q", remark)
	}

	// Empty form_data falls back to defaults.
	scores, totalScore, content, remark = deriveSummary(nil)
	if scores != "{}" || totalScore != nil || content != "" || remark != "" {
		t.Fatalf("empty form_data defaults wrong: %s %v %q %q", scores, totalScore, content, remark)
	}
}
