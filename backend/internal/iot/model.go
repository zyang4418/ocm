package iot

import (
	"context"
	"encoding/json"
	"time"
)

// Device status vocabulary. pending = reported by a source but not yet
// claimed by an operator; online/offline describe presence of an approved
// device (state messages flip online, a source will-message or the stale
// sweep flips offline).
const (
	StatusPending = "pending"
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// GenericCategory is the device category applied when the source does not
// declare one in its state payload. Operators can re-categorize via PATCH.
const GenericCategory = "generic"

// Controlled vocabulary for command types (the things:control whitelist).
// Values are stored in English; the frontend maps them to Chinese labels and
// renders known types as dedicated buttons. A deployment's drivers decide
// which of these their devices actually accept; unknown types are rejected at
// the API so typos can never reach the broker.
const (
	CmdDoorOpen        = "door_open"
	CmdDoorClose       = "door_close"
	CmdSceneStartClass = "scene_start_class"
	CmdSceneEndClass   = "scene_end_class"
)

// CommandTypes is the command whitelist enforced by POST /commands.
var CommandTypes = map[string]bool{
	CmdDoorOpen:        true,
	CmdDoorClose:       true,
	CmdSceneStartClass: true,
	CmdSceneEndClass:   true,
}

// Command lifecycle. queued = accepted and persisted; delivered = the broker
// accepted the publish; acked/failed = the device reported an outcome; expired
// = the deadline passed before an outcome arrived. The expiry sweep and acks
// are the only writers besides these transitions, all guarded by status
// predicates in the UPDATE ... WHERE clauses (see store.go).
const (
	CmdStatusQueued    = "queued"
	CmdStatusDelivered = "delivered"
	CmdStatusAcked     = "acked"
	CmdStatusFailed    = "failed"
	CmdStatusExpired   = "expired"
)

// Command TTL bounds. Devices are expected to act (or fail) quickly; a
// command whose deadline passes is expired rather than delivered late, so a
// door-open command queued during an outage can never fire on reconnect.
const (
	DefaultCommandTTL = 30 * time.Second
	MinCommandTTL     = 1 * time.Second
	MaxCommandTTL     = 10 * time.Minute
)

// ClampCommandTTL normalizes the caller-supplied expires_in_seconds: zero or
// negative falls back to the default, then the result is clamped to
// [MinCommandTTL, MaxCommandTTL].
func ClampCommandTTL(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultCommandTTL
	}
	d := time.Duration(seconds) * time.Second
	if d < MinCommandTTL {
		return MinCommandTTL
	}
	if d > MaxCommandTTL {
		return MaxCommandTTL
	}
	return d
}

// Device is one physical/virtual device as seen by the registry. Identity is
// (site, sourceId, externalId): the publisher's stable external identifier is
// the primary key from the data plane's point of view, and the numeric ID is
// the console/API handle. classroom_id is a logical reference to
// classrooms.id (no FK, matching the repo convention).
//
// State is the latest attribute object reported by the device (retained MQTT
// state payload), replaced wholesale on every report — the registry keeps the
// latest value, not a time series.
type Device struct {
	ID          int64           `json:"id"`
	Site        string          `json:"site"`
	SourceID    string          `json:"sourceId"`
	ExternalID  string          `json:"externalId"`
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	ClassroomID *int64          `json:"classroomId" validate:"optional"`
	Status      string          `json:"status"`
	State       json.RawMessage `json:"state" swaggertype:"object" validate:"optional"`
	LastSeenAt  *time.Time      `json:"lastSeenAt" validate:"optional"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// DeviceEvent is an async device-reported event (alarm, state anomaly).
// occurredAt carries the device's event time, receivedAt the backend's arrival
// time — during offline buffering the two can differ by hours. Duplicate
// deliveries (QoS1 at-least-once) are collapsed by the dedup unique index.
type DeviceEvent struct {
	ID         int64           `json:"id"`
	DeviceID   int64           `json:"deviceId"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload" swaggertype:"object" validate:"optional"`
	OccurredAt time.Time       `json:"occurredAt"`
	ReceivedAt time.Time       `json:"receivedAt"`
}

// DeviceCommand is one control command with its lifecycle record. issuedBy is
// the operator username from the audited REST call.
type DeviceCommand struct {
	ID        int64           `json:"id"`
	CommandID string          `json:"commandId"`
	DeviceID  int64           `json:"deviceId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload" swaggertype:"object" validate:"optional"`
	Status    string          `json:"status"`
	Detail    string          `json:"detail"`
	IssuedBy  string          `json:"issuedBy"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

// ApproveInput claims a pending device. Both fields are required: an
// unclaimed device without a name or classroom binding is exactly the state
// approve exists to end.
type ApproveInput struct {
	Name        string `json:"name"`
	ClassroomID int64  `json:"classroomId"`
	Category    string `json:"category"`
}

// DeviceUpdateInput is the partial-update body for PATCH. Nil fields are left
// unchanged; ClassroomID of 0 clears the binding (a device may legitimately
// not live in any classroom).
type DeviceUpdateInput struct {
	Name        *string `json:"name" validate:"optional"`
	Category    *string `json:"category" validate:"optional"`
	ClassroomID *int64  `json:"classroomId" validate:"optional"`
}

// CommandInput is the body of POST /commands. Payload must be a JSON object
// when present. ExpiresInSeconds is clamped to [1s, 10m]; zero means the
// default (30s). Safety-critical commands (door_open) should stay well under
// the default — see the spec's QoS/expiry matrix.
type CommandInput struct {
	Type             string          `json:"type"`
	Payload          json.RawMessage `json:"payload" swaggertype:"object" validate:"optional"`
	ExpiresInSeconds int             `json:"expiresInSeconds" validate:"optional"`
}

// DeviceFilter carries the optional list filters for PageDevices. Zero values
// are ignored.
type DeviceFilter struct {
	Status      string
	SourceID    string
	Category    string
	ClassroomID int64
}

// CommandFilter carries the optional list filters for PageCommands.
type CommandFilter struct {
	Status   string
	DeviceID int64
}

// CommandPublisher is the seam between the registry and the MQTT data plane.
// The handler only issues commands when a publisher is wired; shipping nil
// (no IOT_MQTT_URL) keeps the registry fully functional and degrades command
// issuing to 503, mirroring the observation Renderer nil pattern.
type CommandPublisher interface {
	PublishCommand(ctx context.Context, d Device, cmd DeviceCommand) error
}
