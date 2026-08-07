package importer

import "time"

// Controlled vocabulary for import job status.
//
//	pending     - created, not yet processed
//	processing  - worker running (parsing or committing)
//	preview     - parsed and validated, awaiting user review before commit
//	succeeded   - committed (may still have per-row failures in FailedRows)
//	failed      - could not parse/commit (system error or unparseable CSV)
//	cancelled   - user discarded the preview without committing
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusPreview    = "preview"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

// JobType identifies what a job imports. Only "sessions" for now; kept as a
// column so future entity imports share the table.
const JobTypeSessions = "sessions"

// CSV column names for the sessions import. The parser maps columns by header
// name, so column order in the file does not matter.
const (
	ColDate        = "date"
	ColPeriodIndex = "period_index"
	ColClassroom   = "classroom"
	ColCourse      = "course"
	ColClass       = "class"
	ColSemester    = "semester"
	ColNote        = "note"
)

// Job is an import task record. Payload holds the raw CSV text; it is excluded
// from JSON responses (large, and only needed server-side while processing).
// Rows holds the successfully parsed rows shown in the preview (empty once the
// job is committed or cancelled).
type Job struct {
	ID            int64        `json:"id"`
	Type          string       `json:"type"`
	Status        string       `json:"status"`
	Filename      string       `json:"filename"`
	Payload       string       `json:"-"`
	TotalRows     int          `json:"totalRows"`
	SucceededRows int          `json:"succeededRows"`
	FailedRows    int          `json:"failedRows"`
	ErrorReport   string       `json:"errorReport"`
	Rows          []PreviewRow `json:"rows,omitempty"`
	UserID        int64        `json:"userId"`
	CreatedAt     time.Time    `json:"createdAt"`
	StartedAt     *time.Time   `json:"startedAt,omitempty"`
	FinishedAt    *time.Time   `json:"finishedAt,omitempty"`
}

// PreviewRow is a successfully parsed, validated row awaiting the user's
// confirmation before it is inserted. It carries the human-readable values
// (names, not resolved IDs) so the operator can review what would be imported.
type PreviewRow struct {
	Date        string `json:"date"`
	PeriodIndex int    `json:"periodIndex"`
	Classroom   string `json:"classroom"`
	Course      string `json:"course"`
	Class       string `json:"class"`
	Semester    string `json:"semester"`
	Note        string `json:"note"`
}

type RowError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// Result is the outcome of processing a CSV import, persisted onto the job.
type Result struct {
	TotalRows     int          `json:"totalRows"`
	SucceededRows int          `json:"succeededRows"`
	FailedRows    int          `json:"failedRows"`
	Errors        []RowError   `json:"errors"`
	Rows          []PreviewRow `json:"rows,omitempty"`
}
