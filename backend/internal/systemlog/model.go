package systemlog

import "time"

// Entry is one audit row to be written by the audit middleware or an explicit
// Record call (auth login, async importer completion).
type Entry struct {
	ActorID    int64 // 0 when there is no attributable actor (failed logins)
	ActorName  string
	Method     string
	Path       string
	StatusCode int
	Summary    string // optional Chinese business description
	ClientIP   string
}

// LogView is the JSON shape returned by the list endpoint.
type LogView struct {
	ID         int64     `json:"id"`
	ActorID    *int64    `json:"actorId"` // null when the row has no actor
	ActorName  string    `json:"actorName"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"statusCode"`
	Summary    string    `json:"summary"`
	ClientIP   string    `json:"clientIp"`
	CreatedAt  time.Time `json:"createdAt"`
}

// LogFilter bounds the list by creation day. From/To are YYYY-MM-DD; the
// empty string means "no bound". To includes the whole day (same semantics as
// the booking filter).
type LogFilter struct {
	From string
	To   string
}

// Settings is the admin-editable log retention policy.
type Settings struct {
	RetentionEnabled bool `json:"retentionEnabled"`
	RetentionDays    int  `json:"retentionDays"`
}
