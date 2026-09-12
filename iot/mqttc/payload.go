// Package mqttc is the device-side SDK of the OCM IoT data plane: it speaks
// the canonical MQTT contract (see iot/spec/spec.md) on behalf of drivers,
// gateways and classroom controllers. It handles connection/reconnection,
// latest-state replay, a persistent FIFO buffer for events/acks, and command
// subscription with ack bookkeeping — so a driver only implements its own
// protocol.
//
// NOTE: payload/topic types here intentionally mirror
// backend/internal/iot/mqtt instead of being imported. The backend module
// cannot import this module (docker build contexts are per-directory), so the
// spec + e2e integration test are the guards against drift.
package mqttc

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PayloadVersion is the schema version stamped on every envelope. Versions
// live in the payload, never in the topic.
const PayloadVersion = 1

// Topic grammar (spec §2).
const (
	TopicRoot    = "iot"
	MetaSegment  = "_meta"
	ChannelState = "state"
	ChannelEvent = "event"
	ChannelCmd   = "cmd"
	ChannelAck   = "ack"

	// MetaChannelOffline is the will-message channel: when the source's
	// connection drops, the backend marks every device of the source offline
	// (spec §6).
	MetaChannelOffline = "offline"
)

// Ack statuses a device may report.
const (
	AckStatusAcked  = "acked"
	AckStatusFailed = "failed"
)

// ErrBadSourceID is returned at connect time when the source identifier does
// not satisfy the topic segment grammar.
var ErrBadSourceID = errors.New("invalid sourceId: must match [A-Za-z0-9._-]{1,128} and not start with '$'")

// ValidSegment reports whether s is a legal topic identifier segment.
func ValidSegment(s string) bool {
	if s == "" || len(s) > 128 || s[0] == '$' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.':
		default:
			return false
		}
	}
	return true
}

// StatePayload is the retained state report on .../state.
type StatePayload struct {
	V        int             `json:"v"`
	At       int64           `json:"at"`
	Category string          `json:"category,omitempty"`
	Attrs    json.RawMessage `json:"attrs"`
}

// EventPayload is an async device event on .../event.
type EventPayload struct {
	V    int             `json:"v"`
	At   int64           `json:"at"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// AckPayload is the device's outcome report on .../ack.
type AckPayload struct {
	V         int    `json:"v"`
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	At        int64  `json:"at"`
}

// CommandFrame is the backend→device command envelope on .../cmd.
type CommandFrame struct {
	V         int             `json:"v"`
	CommandID string          `json:"command_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	IssuedAt  int64           `json:"issued_at"`
	ExpiresAt int64           `json:"expires_at"`
}

// dataTopic builds iot/{site}/{sourceId}/{deviceId}/{channel}.
func dataTopic(site, sourceID, deviceID, channel string) string {
	return TopicRoot + "/" + site + "/" + sourceID + "/" + deviceID + "/" + channel
}

// metaTopic builds iot/_meta/{sourceId}/{channel}.
func metaTopic(sourceID, channel string) string {
	return TopicRoot + "/" + MetaSegment + "/" + sourceID + "/" + channel
}

func marshalPayload(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("mqttc: marshal payload: %w", err)
	}
	return b, nil
}

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

func millis(t time.Time) int64 { return t.UnixMilli() }
