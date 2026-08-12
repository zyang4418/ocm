package importer

import "time"

// Controlled vocabulary for import job status.
//
//	pending     - created, not yet processed
//	processing  - worker running (parsing or committing)
//	preview     - parsed and validated, awaiting user review before commit
//	succeeded   - committed (may still have per-row failures in FailedRows)
//	failed      - could not parse/commit (system error or unparseable file)
//	cancelled   - user discarded the preview without committing
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusPreview    = "preview"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

// JobType identifies what a job imports. Each value corresponds to a registered
// Importer in the Registry; the upload route dispatches by this string.
const (
	JobTypeSessions        = "sessions"
	JobTypeClassrooms      = "classrooms"
	JobTypeAdminClasses    = "admin_classes"
	JobTypeTeachingClasses = "teaching_classes"
	JobTypeCatalog         = "catalog"
	JobTypeOfferings       = "offerings"
	JobTypeRegimes         = "regimes"
	JobTypeBookings        = "bookings"
)

// Job is an import task record. Payload holds the raw uploaded file content; it
// is excluded from JSON responses (large, and only needed server-side while
// processing). Rows holds the successfully parsed rows shown in the preview
// (empty once the job is committed or cancelled). Rows is a generic map so the
// same table serves every entity type; the frontend renders columns by Type.
type Job struct {
	ID            int64            `json:"id"`
	Type          string           `json:"type"`
	Status        string           `json:"status"`
	Filename      string           `json:"filename"`
	Payload       string           `json:"-"`
	TotalRows     int              `json:"totalRows"`
	SucceededRows int              `json:"succeededRows"`
	FailedRows    int              `json:"failedRows"`
	ErrorReport   string           `json:"errorReport"`
	Rows          []map[string]any `json:"rows,omitempty"`
	UserID        int64            `json:"userId"`
	CreatedAt     time.Time        `json:"createdAt"`
	StartedAt     *time.Time       `json:"startedAt,omitempty"`
	FinishedAt    *time.Time       `json:"finishedAt,omitempty"`
}

type RowError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// Result is the outcome of processing an import, persisted onto the job. Rows is
// the preview (human-readable values, not resolved IDs) shown before commit.
type Result struct {
	TotalRows     int              `json:"totalRows"`
	SucceededRows int              `json:"succeededRows"`
	FailedRows    int              `json:"failedRows"`
	Errors        []RowError       `json:"errors"`
	Rows          []map[string]any `json:"rows,omitempty"`
}
