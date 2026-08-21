// Package observation implements the 听课评课 (class observation & evaluation)
// module of the open-source OCM layer.
//
// It provides the generic, schema-agnostic core: the observation record model,
// its CRUD/submit state machine, section normalization and the export
// orchestration. What it deliberately does NOT encode is the school-specific
// document backend — the form templates, their indicators/limits, and the .docx
// fillers. That knowledge is injected through the Renderer interface (see
// export.go) by a deployment's customization layer, keeping this package
// reusable across any institution's evaluation forms.
package observation

import (
	"encoding/json"
	"errors"
	"time"
)

// Status values for an observation record.
const (
	StatusDraft     = "draft"
	StatusSubmitted = "submitted"
)

// Sentinel errors shared by the store and handler.
var (
	ErrNotFound          = errors.New("observation not found")
	ErrDuplicate         = errors.New("observation already exists for this lesson")
	ErrForbidden         = errors.New("not allowed to modify this observation")
	ErrSubmitted         = errors.New("submitted observation cannot be modified")
	ErrCourseNotFound    = errors.New("course offering not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrClassroomNotFound = errors.New("classroom not found")
	ErrRendererMissing   = errors.New("observation document backend not configured")
)

// Observation is one 听课评课 record. The form payload (FormData) is stored
// opaquely; Scores/TotalScore/Content/Remark are summary columns derived from
// it so list/detail responses can expose them without parsing the whole form.
type Observation struct {
	ID             int64           `json:"id"`
	TemplateType   string          `json:"templateType"`
	OccurrenceID   *int64          `json:"occurrenceId" validate:"optional"` // nullable FK -> course_sessions
	CourseID       int64           `json:"courseId"`                         // FK -> course_offerings
	ClassroomID    *int64          `json:"classroomId" validate:"optional"`  // nullable FK -> classrooms
	ObserverID     int64           `json:"observerId"`                       // FK -> users
	ObserveDate    string          `json:"observeDate"`                      // "YYYY-MM-DD"
	Sections       []int           `json:"sections"`                         // normalized period indices
	Status         string          `json:"status"`                           // draft | submitted
	Scores         json.RawMessage `json:"scores" swaggertype:"object"`
	TotalScore     *float64        `json:"totalScore" validate:"optional"`
	Content        string          `json:"content"`
	FormData       json.RawMessage `json:"formData" swaggertype:"object"`
	CourseSnapshot json.RawMessage `json:"courseSnapshot" swaggertype:"object"`
	IsAnonymous    bool            `json:"isAnonymous"`
	Remark         string          `json:"remark"`
	ExportedAt     *time.Time      `json:"exportedAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// ObservationView augments Observation with joined display fields for list and
// detail responses.
type ObservationView struct {
	Observation
	CourseName        string `json:"courseName"`
	CourseCode        string `json:"courseCode"`
	Teacher           string `json:"teacher"`
	TeachingClassName string `json:"teachingClassName"`
	ClassroomName     string `json:"classroomName"`
	ObserverName      string `json:"observerName"`
}

// ObservationInput carries the mutable fields of a create/update request.
// Scores/TotalScore/Content/Remark and CourseSnapshot are derived server-side,
// never trusted from the client.
type ObservationInput struct {
	TemplateType string          `json:"templateType"`
	OccurrenceID *int64          `json:"occurrenceId" validate:"optional"`
	CourseID     int64           `json:"courseId"`
	ClassroomID  *int64          `json:"classroomId" validate:"optional"`
	ObserveDate  string          `json:"observeDate"`
	Sections     []int           `json:"sections"`
	FormData     json.RawMessage `json:"formData" swaggertype:"object"`
	IsAnonymous  bool            `json:"isAnonymous"`
}

// Filter carries the optional list filters. Zero values are ignored.
type Filter struct {
	Status       string // exact status
	TemplateType string // exact template type
	CourseID     int64  // exact course offering
	From         string // "YYYY-MM-DD", observe_date >=
	To           string // "YYYY-MM-DD", observe_date <=
}
