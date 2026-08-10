package course

import "time"

// CatalogCourse is an abstract subject in the course library, reused across
// classes and semesters (e.g. "高等数学").
type CatalogCourse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Offering is a concrete "课程": one teaching class taking one catalog course
// in one semester. The teaching class (教学班) is a named group of admin
// classes, so an offering can span multiple admin classes (合班). Two offerings
// of the same course/teacher/semester taught to different groups are distinct
// teaching classes. It owns its set of course sessions.
type Offering struct {
	ID              int64     `json:"id"`
	CatalogID       int64     `json:"catalogId"`
	TeachingClassID int64     `json:"teachingClassId"`
	Teacher         string    `json:"teacher"`
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
// classroom on a specific date and period. It is the source of truth for the
// real timetable; weekly templates are only a future generation aid.
type Session struct {
	ID          int64     `json:"id"`
	OfferingID  int64     `json:"offeringId"`
	ClassroomID int64     `json:"classroomId"`
	Date        string    `json:"date"` // "YYYY-MM-DD"
	PeriodIndex int       `json:"periodIndex"`
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
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

type OfferingInput struct {
	CatalogID       int64  `json:"catalogId"`
	TeachingClassID int64  `json:"teachingClassId"`
	Teacher         string `json:"teacher"`
	Semester        string `json:"semester"`
	Note            string `json:"note"`
}

type SessionInput struct {
	OfferingID  int64  `json:"offeringId"`
	ClassroomID int64  `json:"classroomId"`
	Date        string `json:"date"`
	PeriodIndex int    `json:"periodIndex"`
	Note        string `json:"note"`
}

// TimetableSlot is one period cell in a classroom timetable day.
type TimetableSlot struct {
	PeriodIndex int          `json:"periodIndex"`
	StartTime   string       `json:"startTime"`
	EndTime     string       `json:"endTime"`
	Session     *SessionView `json:"session"` // nil when the slot is free
}

// TimetableDay is one day column in a classroom timetable grid.
type TimetableDay struct {
	Date       string          `json:"date"`
	DayOfWeek  int             `json:"dayOfWeek"` // 1=Mon .. 7=Sun
	RegimeName string          `json:"regimeName"`
	Slots      []TimetableSlot `json:"slots"`
}
