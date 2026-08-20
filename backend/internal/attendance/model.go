package attendance

import (
	"errors"
	"time"
)

// Record status values. Present/Late come from a student scan (late when the
// scan lands after the checkin's late threshold); Absent/Leave are set by a
// teacher through the records endpoint, and Absent is also derived at query
// time for roster students without a record row.
const (
	StatusActive = "active"
	StatusClosed = "closed"

	StatusPresent = "present"
	StatusAbsent  = "absent"
	StatusLate    = "late"
	StatusLeave   = "leave"
)

var (
	ErrCheckinNotFound  = errors.New("checkin not found")
	ErrCheckinNotActive = errors.New("checkin not active")
	ErrCheckinExpired   = errors.New("checkin expired")
	ErrCodeNotFound     = errors.New("checkin code not found")
	ErrInvalidStatus    = errors.New("invalid record status")
	ErrOfferingMismatch = errors.New("session does not belong to the offering")
	ErrTitleRequired    = errors.New("title required for standalone checkin")
	ErrStudentNotFound  = errors.New("student user not found")
)

// Checkin is one attendance event. OfferingID/SessionID anchor it to the L2/L3
// course hierarchy (0 = standalone checkin not attached to any offering); the
// title carries a human label either way.
type Checkin struct {
	ID          int64      `json:"id"`
	OfferingID  int64      `json:"offeringId"`
	SessionID   int64      `json:"sessionId"`
	Title       string     `json:"title"`
	Code        string     `json:"code"`
	LateMinutes int        `json:"lateMinutes"`
	Status      string     `json:"status"` // active | closed
	StartsAt    time.Time  `json:"startsAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreatedBy   int64      `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	ClosedAt    *time.Time `json:"closedAt"`
}

// CheckinView augments Checkin with the offering/session display names and the
// per-status counts. Status is the effective status: an active checkin past its
// ExpiresAt reports closed.
type CheckinView struct {
	Checkin
	CourseName        string `json:"courseName"`
	TeachingClassName string `json:"teachingClassName"`
	Semester          string `json:"semester"`
	Teacher           string `json:"teacher"`
	SessionText       string `json:"sessionText"`
	Counts            Counts `json:"counts"`
}

// Counts summarizes one checkin. Expected is the roster size (0 for standalone
// checkins); Present/Late/Leave are record counts; Absent is explicit-absent
// records plus roster students without any record row.
type Counts struct {
	Expected int `json:"expected"`
	Present  int `json:"present"`
	Late     int `json:"late"`
	Absent   int `json:"absent"`
	Leave    int `json:"leave"`
}

// CheckinInput is the create payload. DurationMinute>0 sets an ExpiresAt;
// 0 means the checkin stays open until the teacher closes it.
type CheckinInput struct {
	OfferingID     int64  `json:"offeringId"`
	SessionID      int64  `json:"sessionId"`
	Title          string `json:"title"`
	LateMinutes    int    `json:"lateMinutes"`
	DurationMinute int    `json:"durationMinute"`
}

// ScanRequest is the scan body (POST /api/checkins/scan). The handler decodes
// an inline struct of the same shape; this named type backs the OpenAPI schema
// so swaggo can reference it in @Param.
type ScanRequest struct {
	Code string `json:"code" example:"123456"`
}

// RecordUpdateInput is the correction body (PUT /api/checkins/{id}/records/{userId}).
// Like ScanRequest, it documents the inline body the handler decodes.
type RecordUpdateInput struct {
	Status string `json:"status" example:"present" Enums(present,late,absent,leave)`
}

// CheckinRecordView is one student row of a checkin's record list. Rows come
// from two sources: the expected roster (derived via student_profiles → admin
// classes → teaching class of the offering; missing records read as absent)
// and record rows outside the roster (InRoster=false).
type CheckinRecordView struct {
	CheckinID   int64      `json:"checkinId"`
	UserID      int64      `json:"userId"`
	DisplayName string     `json:"displayName"`
	StudentNo   string     `json:"studentNo"`
	AdminClass  string     `json:"adminClass"` // "grade name", "" when unknown
	Status      string     `json:"status"`
	CheckedAt   *time.Time `json:"checkedAt"`
	ModifiedAt  *time.Time `json:"modifiedAt"`
	InRoster    bool       `json:"inRoster"`
}

// ScanResult is the scan response. IsNew=false means the student already had a
// record (Status carries the stored status, which a teacher correction never
// overwrites).
type ScanResult struct {
	CheckinID int64  `json:"checkinId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	IsNew     bool   `json:"isNew"`
	InRoster  bool   `json:"inRoster"`
}

// OfferingSummary is the L2 semester view: every checkin of one offering plus
// one row per student with their per-checkin status and per-status totals.
// Rows covers the expected roster plus any student with a record in one of the
// checkins but outside the roster (InRoster=false, cells blank when absent).
type OfferingSummary struct {
	OfferingID        int64         `json:"offeringId"`
	CourseName        string        `json:"courseName"`
	TeachingClassName string        `json:"teachingClassName"`
	Semester          string        `json:"semester"`
	Teacher           string        `json:"teacher"`
	Checkins          []CheckinView `json:"checkins"`
	Rows              []SummaryRow  `json:"rows"`
}

// MyCheckinView is one entry of a student's own checkin list: the checkin the
// student has a record in, plus the student's record status and time. Only
// checkins with a record appear (absence is not derived here).
type MyCheckinView struct {
	CheckinID         int64      `json:"checkinId"`
	Title             string     `json:"title"`
	CourseName        string     `json:"courseName"`
	TeachingClassName string     `json:"teachingClassName"`
	SessionText       string     `json:"sessionText"`
	StartsAt          time.Time  `json:"startsAt"`
	Status            string     `json:"status"`
	CheckedAt         *time.Time `json:"checkedAt"`
}

// SummaryRow is one student's semester attendance. Records maps checkinID to
// the record status; roster students derive absent where no record exists.
// Totals counts statuses across the offering's checkins.
type SummaryRow struct {
	UserID      int64            `json:"userId"`
	DisplayName string           `json:"displayName"`
	StudentNo   string           `json:"studentNo"`
	AdminClass  string           `json:"adminClass"`
	InRoster    bool             `json:"inRoster"`
	Records     map[int64]string `json:"records"`
	Totals      map[string]int   `json:"totals"`
}
