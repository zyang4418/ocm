package jwc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// parsePeriods 把「节次」展开为逐节序号切片。
//
// 格式：区间 "3-4"/"1-4"/"9-10"，或单节 "1-1"/"2-2"/"3-3"（等价于单节）。
// 返回 [a..b] 升序。空串或非正整数返回错误，便于上层定位到行。
func parsePeriods(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("节次为空")
	}
	parts := strings.SplitN(s, "-", 2)
	a, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || a < 1 {
		return nil, fmt.Errorf("节次格式非法：%q", s)
	}
	if len(parts) == 1 {
		return []int{a}, nil
	}
	b, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || b < a {
		return nil, fmt.Errorf("节次格式非法：%q", s)
	}
	out := make([]int, 0, b-a+1)
	for p := a; p <= b; p++ {
		out = append(out, p)
	}
	return out, nil
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

// sessionCell 是展开后的一个具体课次（L3）：某教室在某日期某节次的一次上课。
type sessionCell struct {
	date        string // "YYYY-MM-DD"
	periodIndex int
	classroom   string
}

// expandSlot 把一个周槽位（教室 + 星期 + 节次集 + 周集）展开为逐个 sessionCell。
// 输出顺序为按周、按节次，便于测试断言与可读性。
func expandSlot(classroom string, weekday int, periods []int, weeks []int, week1Monday time.Time) []sessionCell {
	out := make([]sessionCell, 0, len(weeks)*len(periods))
	for _, w := range weeks {
		d := weekDate(week1Monday, w, weekday)
		for _, p := range periods {
			out = append(out, sessionCell{
				date:        d.Format("2006-01-02"),
				periodIndex: p,
				classroom:   classroom,
			})
		}
	}
	return out
}
