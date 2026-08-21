package course

import "time"

// CatalogCourse is an abstract subject in the course library, reused across
// classes and semesters (e.g. "高等数学"). The extended fields
// (Credits/TotalHours/Category/ExamType) carry the academic attributes that
// accompany a course in the 教务处 export (学分/总学时/课程类别二/考核方式);
// they are optional and default to zero/empty for legacy data.
type CatalogCourse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Credits     float64   `json:"credits"`
	TotalHours  int       `json:"totalHours"`
	Category    string    `json:"category"` // 课程类别二：专业基础课/专业课/学科基础课/通识教育课
	ExamType    string    `json:"examType"` // 考核方式：考试/考查
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Offering is a concrete "课程": one teaching class taking one catalog course
// in one semester. The teaching class (教学班) is a named group of admin
// classes, so an offering can span multiple admin classes (合班). Two offerings
// of the same course/teacher/semester taught to different groups are distinct
// teaching classes. It owns its set of course sessions.
//
// The extended fields (CourseSeq/TeacherID/TeacherTitle/College/MaxStudents/
// Requirement/WeeklyHours) carry 教务处 section metadata (课程序号/教师工号/
// 教师职称/开课学院/人数上限/课程类别一/周学时); all optional with defaults so
// legacy data and manual CRUD still work without them.
type Offering struct {
	ID              int64     `json:"id"`
	CatalogID       int64     `json:"catalogId"`
	TeachingClassID int64     `json:"teachingClassId"`
	Teacher         string    `json:"teacher"`
	CourseSeq       string    `json:"courseSeq"`    // 课程序号，如 113130004.68
	TeacherID       string    `json:"teacherId"`    // 教师工号（合上课逗号合并）
	TeacherTitle    string    `json:"teacherTitle"` // 教师职称
	College         string    `json:"college"`      // 开课学院
	MaxStudents     int       `json:"maxStudents"`  // 人数上限
	Requirement     string    `json:"requirement"`  // 课程类别一：必修/限选/任选
	WeeklyHours     int       `json:"weeklyHours"`  // 周学时
	Semester        string    `json:"semester"`
	Note            string    `json:"note"`
	CreatedAt       time.Time `json:"createdAt"`
}

// OfferingView is an offering joined with its catalog course name/code and the
// teaching class display name plus its member admin class names.
type OfferingView struct {
	Offering
	CatalogName       string   `json:"catalogName"`
	CatalogCode       string   `json:"catalogCode"`
	TeachingClassName string   `json:"teachingClassName"`
	ClassNames        []string `json:"classNames"`
}

// Session is one actual class meeting (上课实例): a course offering in a
// classroom on a specific date over a contiguous period range [PeriodStart,
// PeriodEnd]. Back-to-back periods of one meeting are a single session, not
// one row per period. It is the source of truth for the real timetable;
// weekly templates are only a future generation aid.
type Session struct {
	ID          int64     `json:"id"`
	OfferingID  int64     `json:"offeringId"`
	ClassroomID int64     `json:"classroomId"`
	Date        string    `json:"date"` // "YYYY-MM-DD"
	PeriodStart int       `json:"periodStart"`
	PeriodEnd   int       `json:"periodEnd"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SessionView is a session joined with offering, catalog and classroom display
// fields, used in session lists and the classroom timetable.
type SessionView struct {
	Session
	CourseName        string   `json:"courseName"`
	CatalogCode       string   `json:"catalogCode"`
	TeachingClassName string   `json:"teachingClassName"`
	ClassNames        []string `json:"classNames"`
	Teacher           string   `json:"teacher"`
	Semester          string   `json:"semester"`
	ClassroomName     string   `json:"classroomName"`
}

type CatalogInput struct {
	Name        string  `json:"name"`
	Code        string  `json:"code"`
	Credits     float64 `json:"credits"`
	TotalHours  int     `json:"totalHours"`
	Category    string  `json:"category"`
	ExamType    string  `json:"examType"`
	Description string  `json:"description"`
}

type OfferingInput struct {
	CatalogID       int64  `json:"catalogId"`
	TeachingClassID int64  `json:"teachingClassId"`
	Teacher         string `json:"teacher"`
	CourseSeq       string `json:"courseSeq"`
	TeacherID       string `json:"teacherId"`
	TeacherTitle    string `json:"teacherTitle"`
	College         string `json:"college"`
	MaxStudents     int    `json:"maxStudents"`
	Requirement     string `json:"requirement"`
	WeeklyHours     int    `json:"weeklyHours"`
	Semester        string `json:"semester"`
	Note            string `json:"note"`
}

type SessionInput struct {
	OfferingID  int64  `json:"offeringId"`
	ClassroomID int64  `json:"classroomId"`
	Date        string `json:"date"`
	PeriodStart int    `json:"periodStart"`
	PeriodEnd   int    `json:"periodEnd"`
	Note        string `json:"note"`
}

// TimetableSlot is one period cell in a classroom timetable day.
type TimetableSlot struct {
	PeriodIndex int          `json:"periodIndex"`
	StartTime   string       `json:"startTime"`
	EndTime     string       `json:"endTime"`
	Session     *SessionView `json:"session" validate:"optional"` // nil when the slot is free
}

// TimetableDay is one day column in a classroom timetable grid.
type TimetableDay struct {
	Date       string          `json:"date"`
	DayOfWeek  int             `json:"dayOfWeek"` // 1=Mon .. 7=Sun
	RegimeName string          `json:"regimeName"`
	Slots      []TimetableSlot `json:"slots"`
}
