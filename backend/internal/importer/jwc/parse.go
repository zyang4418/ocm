package jwc

import (
	"fmt"
	"strconv"
	"strings"

	"ocm-backend/internal/xlsx"
)

// 教务处源表中文表头（xlsx.MapRows 已对表头做 lower-trim，中文无大小写变化，
// 故直接用中文原文作为列键，顺序无关）。
const (
	hBuilding     = "教学楼"
	hClassroom    = "上课教室"
	hRoomType     = "教室类型"
	hFloor        = "楼层"
	hWeekday      = "星期"
	hPeriod       = "节次"
	hWeeks        = "起止周"
	hCourseSeq    = "课程序号"
	hCourseCode   = "课程代码"
	hCourseName   = "课程名称"
	hAdminClasses = "行政班"
	hTeacherName  = "教师姓名"
	hTeacherTitle = "教师职称"
	hTeacherID    = "教师工号"
	hMaxStudents  = "人数上限"
	hCollege      = "开课学院"
	hRequirement  = "课程类别一"
	hCategory     = "课程类别二"
	hTotalHours   = "总学时"
	hWeeklyHours  = "周学时"
	hCredits      = "学分"
	hExamType     = "考核方式"
	hGrade        = "年级"
	hCampus       = "校区"
	hRoomCapacity = "教室人数"
)

// jwcRow 是教务处源表一行的强类型视图。数值列保留原始字符串由后续阶段按需解析，
// 便于在解析失败时定位到具体行。
type jwcRow struct {
	building     string
	classroom    string
	roomType     string
	floor        string
	weekday      int
	periodStr    string
	weekStr      string
	courseSeq    string
	courseCode   string
	courseName   string
	adminClasses string
	teacherName  string
	teacherTitle string
	teacherID    string
	maxStudents  string
	college      string
	requirement  string
	category     string
	totalHours   string
	weeklyHours  string
	credits      string
	examType     string
	grade        string
	campus       string
	roomCapacity string
	rowNum       int // 1-based 文件行号，用于错误定位
}

// parseRows 读教务处 xlsx 首 sheet，按中文表头映射为 jwcRow。返回的行保留全部源行
// （含空行政班行、解析失败的星期行），由后续阶段决定跳过还是报错。仅当表头缺失或
// 文件无法打开时返回错误。
func parseRows(data []byte) (rows []jwcRow, err error) {
	_, recs, err := xlsx.MapRows(data)
	if err != nil {
		return nil, fmt.Errorf("读取教务处 xlsx 失败：%w", err)
	}
	for i, rec := range recs {
		rows = append(rows, jwcRow{
			building:     rec[hBuilding],
			classroom:    rec[hClassroom],
			roomType:     rec[hRoomType],
			floor:        rec[hFloor],
			weekday:      atoiOr(rec[hWeekday], 0),
			periodStr:    rec[hPeriod],
			weekStr:      rec[hWeeks],
			courseSeq:    rec[hCourseSeq],
			courseCode:   rec[hCourseCode],
			courseName:   rec[hCourseName],
			adminClasses: rec[hAdminClasses],
			teacherName:  rec[hTeacherName],
			teacherTitle: rec[hTeacherTitle],
			teacherID:    rec[hTeacherID],
			maxStudents:  rec[hMaxStudents],
			college:      rec[hCollege],
			requirement:  rec[hRequirement],
			category:     rec[hCategory],
			totalHours:   rec[hTotalHours],
			weeklyHours:  rec[hWeeklyHours],
			credits:      rec[hCredits],
			examType:     rec[hExamType],
			grade:        rec[hGrade],
			campus:       rec[hCampus],
			roomCapacity: rec[hRoomCapacity],
			rowNum:       i + 2, // MapRows 已去掉表头；+1 表头 +1 转 1-based
		})
	}
	return rows, nil
}

// atoiOr 解析整型，空或非法返回 def。jwc 包不依赖 importer 包，故本地提供。
func atoiOr(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// atofOr 解析浮点，空或非法返回 def。
func atofOr(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return n
}
