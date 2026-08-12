package jwc

import (
	"sort"
	"strconv"
	"strings"
)

// maxNameLen 是 teaching_classes.name 列的长度上限（VARCHAR(64)）。压缩命名超长时
// 回退到「首班,次班等N班」，完整成员写入 note。
const maxNameLen = 64

// parseAdminSet 把「行政班」单元格拆为成员切片（去空白、去重、保首次出现顺序）。
// 任选课的行政班为空 -> ok=false，由上层决定跳过。
func parseAdminSet(s string) (members []string, ok bool) {
	seen := make(map[string]bool)
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		members = append(members, p)
	}
	return members, len(members) > 0
}

// adminSetKey 返回行政班集合的规范键（排序后逗号连接），用于教学班去重与平行教学班
// 检测：成员相同但顺序不同的两行应映射到同一教学班。
func adminSetKey(members []string) string {
	cp := append([]string(nil), members...)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}

// teachingClassName 由成员列表合成教学班名。
//
// 算法：按 (前缀, 末尾数字) 分组；同组连续编号压缩为「前缀首号~尾号」（机电241~245）；
// 单个或断裂则原样列出；组间用「,」连接。若结果超 64 字符则回退「首班,次班等N班」，
// 并把完整成员列表写入返回的 note（回退时非空，否则为空）。
func teachingClassName(members []string) (name, note string) {
	name = compressName(members)
	if len([]rune(name)) <= maxNameLen {
		return name, ""
	}
	n := len(members)
	fb := members[0] + "," + members[1] + "等" + strconv.Itoa(n) + "班"
	return fb, strings.Join(members, ",")
}

// compressName 实现教学班命名的区间压缩。成员按前缀分组（首次出现序），组内按数字
// 升序；连续段压缩，非连续或无数字者原样保留。
func compressName(members []string) string {
	type entry struct {
		num    int
		hasNum bool
		name   string
	}
	order := []string{}
	groups := map[string][]entry{}
	for _, m := range members {
		pfx, digits := splitTrailingDigits(m)
		if _, ok := groups[pfx]; !ok {
			order = append(order, pfx)
		}
		if digits == "" {
			groups[pfx] = append(groups[pfx], entry{name: m})
		} else {
			n, _ := strconv.Atoi(digits)
			groups[pfx] = append(groups[pfx], entry{num: n, hasNum: true, name: m})
		}
	}
	var parts []string
	for _, pfx := range order {
		es := groups[pfx]
		var nums, nonums []entry
		for _, e := range es {
			if e.hasNum {
				nums = append(nums, e)
			} else {
				nonums = append(nonums, e)
			}
		}
		sort.SliceStable(nums, func(i, j int) bool { return nums[i].num < nums[j].num })
		for i := 0; i < len(nums); i++ {
			start := i
			for i+1 < len(nums) && nums[i+1].num == nums[i].num+1 {
				i++
			}
			if i > start {
				parts = append(parts, pfx+strconv.Itoa(nums[start].num)+"~"+strconv.Itoa(nums[i].num))
			} else {
				parts = append(parts, nums[start].name)
			}
		}
		for _, e := range nonums {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, ",")
}

// splitTrailingDigits 把班名拆为 (前缀, 末尾最长数字串)。
// "工设UC-241" -> ("工设UC-", "241")；"人工智能241" -> ("人工智能", "241")。
func splitTrailingDigits(s string) (prefix, digits string) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[:i], s[i:]
}

// gradeFor 推导行政班的入学年级（4 位年份）。
// 优先用教务处「年级」列的值（权威）；为空时由班名末尾数字串前两位推导（241->2024）。
func gradeFor(name, source string) string {
	if s := strings.TrimSpace(source); s != "" {
		return s
	}
	_, digits := splitTrailingDigits(name)
	if len(digits) < 2 {
		return ""
	}
	return "20" + digits[:2]
}
