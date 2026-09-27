package jwc

import "testing"

// 「星期」严格解析：1-7 合法，空/越界/非数字报错（消息带原文），与 parsePeriods 同风格。
func TestParseWeekday(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"1", 1, true},
		{"7", 7, true},
		{" 3 ", 3, true},
		{"", 0, false},
		{"0", 0, false},
		{"8", 0, false},
		{"星期五", 0, false},
	}
	for _, c := range cases {
		got, err := parseWeekday(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("parseWeekday(%q) = %d, %v；期望 %d", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("parseWeekday(%q) 应报错，got %d", c.in, got)
		}
	}
}
