package classroom

import (
	"encoding/json"
	"errors"
	"time"
)

// Repair status values. A ticket flows open -> processing -> completed ->
// confirmed; the transitions and who may advance them are enforced by the
// store (see repair_store.go). Repair state is independent of the classroom's
// own status: filing or resolving a ticket never mutates the classroom row.
const (
	RepairStatusOpen       = "open"
	RepairStatusProcessing = "processing"
	RepairStatusCompleted  = "completed"
	RepairStatusConfirmed  = "confirmed"
)

// Sentinel errors shared by the repair store and handler.
var (
	ErrRepairNotFound   = errors.New("repair ticket not found")
	ErrRepairForbidden  = errors.New("not allowed to modify this repair ticket")
	ErrRepairOpenExists = errors.New("an open repair ticket already exists for this classroom")
	ErrRepairState      = errors.New("repair ticket is not in a state that allows this action")
)

// Repair is one classroom repair ticket. It references a classroom but never
// mutates it. Images is a reserved JSON list of URLs: it round-trips verbatim
// but nothing uploads into it yet (object storage upload is unimplemented).
type Repair struct {
	ID          int64           `json:"id"`
	ClassroomID int64           `json:"classroomId"`
	CreatorID   int64           `json:"creatorId"`
	AssigneeID  *int64          `json:"assigneeId" validate:"optional"`
	Description string          `json:"description"`
	Images      json.RawMessage `json:"images" swaggertype:"array,string"`
	Status      string          `json:"status"`
	Remark      string          `json:"remark"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// RepairView augments Repair with joined display fields for list and detail.
type RepairView struct {
	Repair
	ClassroomName string `json:"classroomName"`
	Building      string `json:"building"`
	CreatorName   string `json:"creatorName"`
	AssigneeName  string `json:"assigneeName"`
}

// RepairInput carries the mutable fields of a create request.
type RepairInput struct {
	ClassroomID int64           `json:"classroomId"`
	Description string          `json:"description"`
	Images      json.RawMessage `json:"images" swaggertype:"array,string"`
}

// RepairUpdateInput carries the assignee-owned status transition. Status is
// limited to processing (start) or completed (finish); confirmation is a
// separate creator-owned action, not a status write.
type RepairUpdateInput struct {
	Status string `json:"status"`
	Remark string `json:"remark"`
}

// RepairFilter carries the optional list filters. Zero values are ignored.
// Statuses matches ANY of the listed statuses (used by the dashboard's
// "unresolved" view: open OR processing); Status matches exactly one.
type RepairFilter struct {
	ClassroomID int64
	Status      string
	Statuses    []string
}
