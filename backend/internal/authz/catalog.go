package authz

import "sort"

// Permission is one entry of the permission catalog. Code is the stable
// contract checked by handlers; Name/Category/CategoryName/Description exist
// for console display and grouping. The catalog is defined in code and the
// database stores only the permission strings.
type Permission struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	CategoryName string `json:"categoryName"`
	Description  string `json:"description"`
}

// Catalog is the full list of permissions, sorted by category then code so
// the JSON output is stable. The wildcard "*" is intentionally absent: it
// can only ever come from the seeded admin role, never from API input.
var Catalog = sortedCatalog()

func sortedCatalog() []Permission {
	perms := []Permission{
		{Code: UserManage, Name: "用户管理", Category: "user", CategoryName: "用户管理",
			Description: "创建、编辑、删除用户账号，重置密码，授予或撤销角色与权限"},
		{Code: UserRead, Name: "查看用户", Category: "user", CategoryName: "用户管理",
			Description: "查看用户列表、详情与授权情况"},
		{Code: RoleRead, Name: "查看角色", Category: "iam", CategoryName: "权限角色",
			Description: "查看角色与权限目录"},
		{Code: RoleManage, Name: "角色管理", Category: "iam", CategoryName: "权限角色",
			Description: "创建、编辑、删除角色及角色权限分配"},
		{Code: GroupRead, Name: "查看用户组", Category: "iam", CategoryName: "权限角色",
			Description: "查看用户组及其成员、角色"},
		{Code: GroupManage, Name: "用户组管理", Category: "iam", CategoryName: "权限角色",
			Description: "创建、编辑、删除用户组，维护组成员与组角色"},
		{Code: ClassroomRead, Name: "查看教室", Category: "classroom", CategoryName: "教室",
			Description: "查看教室列表与详情、导出"},
		{Code: ClassroomManage, Name: "教室管理", Category: "classroom", CategoryName: "教室",
			Description: "创建、编辑、删除教室，导入教室数据"},
		{Code: ClassroomBook, Name: "预约教室", Category: "classroom", CategoryName: "教室",
			Description: "创建与取消教室预约"},
		{Code: BookingApprove, Name: "审批预约", Category: "booking", CategoryName: "预约",
			Description: "审批、驳回预约；导入预约数据"},
		{Code: CourseRead, Name: "查看课程", Category: "course", CategoryName: "课程",
			Description: "查看课程库、开课、课次、作息与课表"},
		{Code: CourseManage, Name: "课程管理", Category: "course", CategoryName: "课程",
			Description: "维护课程库、开课、课次、作息、课表及课程相关导入"},
		{Code: AdminClassRead, Name: "查看行政班", Category: "admin_class", CategoryName: "行政班",
			Description: "查看行政班"},
		{Code: AdminClassManage, Name: "行政班管理", Category: "admin_class", CategoryName: "行政班",
			Description: "维护行政班，导入行政班数据"},
		{Code: TeachingClassRead, Name: "查看教学班", Category: "teaching_class", CategoryName: "教学班",
			Description: "查看教学班"},
		{Code: TeachingClassManage, Name: "教学班管理", Category: "teaching_class", CategoryName: "教学班",
			Description: "维护教学班，导入教学班数据"},
		{Code: RepairCreate, Name: "提交报修", Category: "repair", CategoryName: "报修",
			Description: "提交教室报修"},
		{Code: RepairAssign, Name: "指派报修", Category: "repair", CategoryName: "报修",
			Description: "指派处理报修"},
		{Code: LogRead, Name: "查看日志", Category: "log", CategoryName: "系统日志",
			Description: "查看系统操作日志与日志保留设置"},
		{Code: LogManage, Name: "日志管理", Category: "log", CategoryName: "系统日志",
			Description: "修改日志保留策略（开关与保留天数）"},
		{Code: AiChat, Name: "AI 助手", Category: "ai", CategoryName: "AI 助手",
			Description: "与 AI 助手对话，查询教室、空闲教室与课表，并生成预约方案（需人工确认）"},
		{Code: AttendanceRead, Name: "查看签到", Category: "attendance", CategoryName: "课堂签到",
			Description: "查看签到活动、签到记录与整学期考勤报表"},
		{Code: AttendanceManage, Name: "签到管理", Category: "attendance", CategoryName: "课堂签到",
			Description: "发起与结束签到、修改学生签到状态、维护学生档案、导出报表"},
		{Code: AttendanceCheckin, Name: "扫码签到", Category: "attendance", CategoryName: "课堂签到",
			Description: "扫码或输入 6 位签到码完成签到"},
		{Code: ObservationRead, Name: "查看评课", Category: "observation", CategoryName: "听课评课",
			Description: "查看自己提交的听课评课记录"},
		{Code: ObservationWrite, Name: "填写评课", Category: "observation", CategoryName: "听课评课",
			Description: "创建、编辑、提交、删除自己的听课评课记录"},
		{Code: ObservationManage, Name: "评课管理", Category: "observation", CategoryName: "听课评课",
			Description: "查看全部听课评课记录并导出任意已提交的评课表"},
	}
	sort.Slice(perms, func(i, j int) bool {
		if perms[i].Category != perms[j].Category {
			return perms[i].Category < perms[j].Category
		}
		return perms[i].Code < perms[j].Code
	})
	return perms
}

// PermissionExists reports whether code is a known catalog permission. The
// wildcard "*" is not part of the catalog, so this returns false for it and
// role/user permission validation naturally rejects the wildcard.
func PermissionExists(code string) bool {
	for _, p := range Catalog {
		if p.Code == code {
			return true
		}
	}
	return false
}
