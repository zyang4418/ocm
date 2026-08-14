package jwc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// parsePeriods 把「节次」解析为连续区间 [start, end]。
//
// 格式：区间 "3-4"/"1-4"/"9-10"，或单节 "1-1"/"2-2"/"3-3"（等价于单节）。
// 连上多节（如 "3-4"）保持为一个区间而不是逐节展开：同一次上课实例占一节与
// 占两节是同一件事。空串或非正整数返回错误，便于上层定位到行。
func parsePeriods(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("节次为空")
	}
	parts := strings.SplitN(s, "-", 2)
	a, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || a < 1 {
		return 0, 0, fmt.Errorf("节次格式非法：%q", s)
	}
	if len(parts) == 1 {
		return a, a, nil
	}
	b, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || b < a {
		return 0, 0, fmt.Errorf("节次格式非法：%q", s)
	}
	return a, b, nil
}

// expandWeeks 把「起止周」展开为具体周次集合（升序去重）。
//
// 格式：逗号分隔的段，每段为 [a-b] 或 a-b（区间）或 n（单周），可带「单」/「双」
// 后缀做奇偶过滤（单=奇数周，双=偶数周）。
// 例："[1-3]单,[4-6]双,[7-17]" -> {1,3,5,7..17}（4-6 取偶数 4,6）。
// 空串或非正整数返回错误。
func expandWeeks(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("起止周为空")
	}
	set := make(map[int]bool)
	for _, seg := range strings.Split(s, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		// 奇偶后缀：单=奇数周，双=偶数周。
		parity := 0
		if strings.HasSuffix(seg, "单") {
			parity = 1
			seg = strings.TrimSpace(strings.TrimSuffix(seg, "单"))
		} else if strings.HasSuffix(seg, "双") {
			parity = 2
			seg = strings.TrimSpace(strings.TrimSuffix(seg, "双"))
		}
		// 方括号可选：[1-3] -> 1-3。
		seg = strings.Trim(seg, "[]")
		seg = strings.TrimSpace(seg)
		var a, b int
		if i := strings.Index(seg, "-"); i >= 0 {
			x, err1 := strconv.Atoi(strings.TrimSpace(seg[:i]))
			y, err2 := strconv.Atoi(strings.TrimSpace(seg[i+1:]))
			if err1 != nil || err2 != nil || x < 1 || y < x {
				return nil, fmt.Errorf("起止周格式非法：%q", s)
			}
			a, b = x, y
		} else {
			x, err := strconv.Atoi(seg)
			if err != nil || x < 1 {
				return nil, fmt.Errorf("起止周格式非法：%q", s)
			}
			a, b = x, x
		}
		for w := a; w <= b; w++ {
			if parity == 1 && w%2 == 0 {
				continue
			}
			if parity == 2 && w%2 == 1 {
				continue
			}
			set[w] = true
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("起止周为空：%q", s)
	}
	out := make([]int, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	sort.Ints(out)
	return out, nil
}

// weekDate 把 (周次, 星期) 换算为具体日期。
//
// week1Monday 为第一周周一；weekday 1=周一..7=周日。
// date = week1Monday + (week-1)*7 + (weekday-1)。
func weekDate(week1Monday time.Time, week, weekday int) time.Time {
	return week1Monday.AddDate(0, 0, (week-1)*7+(weekday-1))
}

// sessionCell 是展开后的一个具体课次（L3）：某教室在某日期的一次上课，占连续
// 节次区间 [periodStart, periodEnd]（连上多节记为一个实例，不逐节拆分）。
type sessionCell struct {
	date        string // "YYYY-MM-DD"
	periodStart int
	periodEnd   int
	classroom   string
}

// expandSlot 把一个周槽位（教室 + 星期 + 节次区间 + 周集）展开为逐个 sessionCell：
// 每周一个课次，节次保持区间。输出顺序为按周升序，便于测试断言与可读性。
func expandSlot(classroom string, weekday int, periodStart, periodEnd int, weeks []int, week1Monday time.Time) []sessionCell {
	out := make([]sessionCell, 0, len(weeks))
	for _, w := range weeks {
		d := weekDate(week1Monday, w, weekday)
		out = append(out, sessionCell{
			date:        d.Format("2006-01-02"),
			periodStart: periodStart,
			periodEnd:   periodEnd,
			classroom:   classroom,
		})
	}
	return out
}
