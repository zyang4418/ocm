package booking

import "time"

// Controlled vocabulary for booking status. Values are stored in English; the
// frontend maps them to Chinese labels. pending and approved occupy the
// classroom's period slot (participate in conflict detection); rejected and
// cancelled do not.
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// Booking is one reservation request: a user reserving a classroom for a
// contiguous range of bell-time periods on a single date. It is period-grid
// aligned, mirroring course_sessions so the two share a unified conflict model
// on (classroom_id, date, period).
type Booking struct {
	ID          int64      `json:"id"`
	ClassroomID int64      `json:"classroomId"`
	UserID      int64      `json:"userId"`
	Date        string     `json:"date"` // "YYYY-MM-DD"
	PeriodStart int        `json:"periodStart"`
	PeriodEnd   int        `json:"periodEnd"`
	Status      string     `json:"status"`
	Purpose     string     `json:"purpose"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReviewedAt  *time.Time `json:"reviewedAt,omitempty"`
}

// BookingView is a booking joined with classroom and booker display fields,
// used in list and detail responses.
type BookingView struct {
	Booking
	ClassroomName string `json:"classroomName"`
	Username      string `json:"username"`
	DisplayName   string `json:"displayName"`
}

// BookingInput carries the mutable fields for a create request. UserID is taken
// from the authenticated subject, not the request body.
type BookingInput struct {
	ClassroomID int64  `json:"classroomId"`
	Date        string `json:"date"`
	PeriodStart int    `json:"periodStart"`
	PeriodEnd   int    `json:"periodEnd"`
	Purpose     string `json:"purpose"`
}

// ReviewInput carries an admin's approve/reject decision on a pending booking.
type ReviewInput struct {
	Decision string `json:"decision"` // "approve" or "reject"
}
