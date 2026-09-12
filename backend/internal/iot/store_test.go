package iot

import (
	"encoding/json"
	"testing"
	"time"
)

// EventDedupHash must be stable for identical deliveries (so the unique index
// collapses QoS1 redeliveries) and differ when any input changes.
func TestEventDedupHash(t *testing.T) {
	at := time.UnixMilli(1730000000000)
	payload := []byte(`{"code":"CHAN_0001"}`)

	h1 := EventDedupHash("door.forced", at, payload)
	if h2 := EventDedupHash("door.forced", at, payload); h1 != h2 {
		t.Fatalf("hash not stable: %q vs %q", h1, h2)
	}
	if len(h1) != 16 {
		t.Fatalf("hash must be 16 hex chars, got %q", h1)
	}
	distinct := []struct {
		name    string
		typ     string
		at      time.Time
		payload []byte
	}{
		{"different type", "door.open", at, payload},
		{"different time", "door.forced", at.Add(time.Millisecond), payload},
		{"different payload", "door.forced", at, []byte(`{"code":"CHAN_0002"}`)},
	}
	for _, tc := range distinct {
		if h := EventDedupHash(tc.typ, tc.at, tc.payload); h == h1 {
			t.Fatalf("hash collision for %s: %q", tc.name, h)
		}
	}
}

func TestClampCommandTTL(t *testing.T) {
	cases := []struct {
		in   int
		want time.Duration
	}{
		{in: 0, want: DefaultCommandTTL},  // unset → default
		{in: -5, want: DefaultCommandTTL}, // nonsense → default
		{in: 10, want: 10 * time.Second},  // door-class commands
		{in: 1, want: MinCommandTTL},      // clamped up to the floor
		{in: 3600, want: MaxCommandTTL},   // clamped down to the ceiling
		{in: 120, want: 2 * time.Minute},  // in-range passes through
	}
	for _, tc := range cases {
		if got := ClampCommandTTL(tc.in); got != tc.want {
			t.Fatalf("ClampCommandTTL(%d) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestCommandTypeWhitelist(t *testing.T) {
	// The four scene/door commands from the field protocol must stay
	// whitelisted; the map is the things:control gate.
	for _, cmd := range []string{CmdDoorOpen, CmdDoorClose, CmdSceneStartClass, CmdSceneEndClass} {
		if !CommandTypes[cmd] {
			t.Fatalf("command %s missing from whitelist", cmd)
		}
	}
	if CommandTypes["format_disk"] {
		t.Fatal("unexpected command in whitelist")
	}
}

// The wire DTOs must keep their JSON field names — they are generated-client
// contract (web/src/types/api.d.ts).
func TestDeviceWireShape(t *testing.T) {
	classroom := int64(3)
	d := Device{ID: 1, Site: "main", SourceID: "gw01", ExternalID: "C301-door",
		Name: "C301 门磁", Category: "door_sensor", ClassroomID: &classroom,
		Status: StatusOnline, State: json.RawMessage(`{"open":false}`)}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "site", "sourceId", "externalId", "name", "category", "classroomId", "status", "state"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("Device JSON missing key %q", key)
		}
	}
	if m["sourceId"] != "gw01" || m["externalId"] != "C301-door" {
		t.Fatalf("unexpected camelCase values: %v", m)
	}
}

func TestCommandWireShape(t *testing.T) {
	c := DeviceCommand{ID: 1, CommandID: "ab12cd34ab12cd34ab12cd34ab12cd34", DeviceID: 1,
		Type: CmdDoorOpen, Status: CmdStatusQueued, IssuedBy: "admin"}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"commandId", "deviceId", "type", "status", "issuedBy", "expiresAt"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("DeviceCommand JSON missing key %q", key)
		}
	}
}
