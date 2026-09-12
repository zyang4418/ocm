package mqtt

import (
	"encoding/json"
	"fmt"
	"time"
)

// The wire payloads of the OCM IoT data plane. Every envelope carries "v": 1
// (schema version lives in the payload, never in the topic) and "at" (event
// time as Unix milliseconds UTC) so buffered deliveries keep their original
// timestamps after reconnection.

const PayloadVersion = 1

// Ack statuses a device may report (backend-settled statuses — delivered,
// expired — are never accepted from the wire).
const (
	AckStatusAcked  = "acked"
	AckStatusFailed = "failed"
)

// StatePayload is the retained state report on .../state. Attrs must be a
// JSON object (or empty — an empty report just marks the device seen).
// Category is the source's declaration of what the device is; the registry
// only honors it on first sight.
type StatePayload struct {
	V        int             `json:"v"`
	At       int64           `json:"at"`
	Category string          `json:"category,omitempty"`
	Attrs    json.RawMessage `json:"attrs"`
}

// Validate enforces the state contract.
func (p *StatePayload) Validate() error {
	if p.V != PayloadVersion {
		return fmt.Errorf("unsupported payload version %d", p.V)
	}
	if p.At <= 0 {
		return fmt.Errorf("missing event time")
	}
	if len(p.Attrs) > 0 && !isJSONObject(p.Attrs) {
		return fmt.Errorf("attrs must be a JSON object")
	}
	if p.Category != "" && !validEventName(p.Category) {
		return fmt.Errorf("invalid category %q", p.Category)
	}
	return nil
}

// EventPayload is an async device event on .../event (alarm, anomaly).
type EventPayload struct {
	V    int             `json:"v"`
	At   int64           `json:"at"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Validate enforces the event contract. Type is 1-64 printable ASCII
// characters without spaces; Data must be a JSON object when present.
func (p *EventPayload) Validate() error {
	if p.V != PayloadVersion {
		return fmt.Errorf("unsupported payload version %d", p.V)
	}
	if p.At <= 0 {
		return fmt.Errorf("missing event time")
	}
	if !validEventName(p.Type) {
		return fmt.Errorf("invalid event type %q", p.Type)
	}
	if len(p.Data) > 0 && !isJSONObject(p.Data) {
		return fmt.Errorf("data must be a JSON object")
	}
	return nil
}

// AckPayload is the device's outcome report on .../ack.
type AckPayload struct {
	V         int    `json:"v"`
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
	At        int64  `json:"at"`
}

// Validate enforces the ack contract. A 32-hex command_id matches what the
// backend issued; anything else cannot be correlated and is dropped.
func (p *AckPayload) Validate() error {
	if p.V != PayloadVersion {
		return fmt.Errorf("unsupported payload version %d", p.V)
	}
	if len(p.CommandID) != 32 {
		return fmt.Errorf("malformed command_id")
	}
	switch p.Status {
	case AckStatusAcked, AckStatusFailed:
	default:
		return fmt.Errorf("invalid ack status %q", p.Status)
	}
	return nil
}

// CommandFrame is the backend→device command envelope published on .../cmd.
// ExpiresAt is authoritative: the device must drop the command once the
// deadline passes, so a command queued during an outage can never fire on
// reconnect (the spec's safety rule for door/scene commands).
type CommandFrame struct {
	V         int             `json:"v"`
	CommandID string          `json:"command_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	IssuedAt  int64           `json:"issued_at"`
	ExpiresAt int64           `json:"expires_at"`
}

// TimeFromMillis converts a payload event time to time.Time.
func TimeFromMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

// isJSONObject reports whether b is valid JSON encoding of an object.
func isJSONObject(b []byte) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(b, &v) == nil
}

// validEventName: 1-64 printable ASCII characters, no spaces or controls.
func validEventName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] <= 0x20 || s[i] >= 0x7F {
			return false
		}
	}
	return true
}
