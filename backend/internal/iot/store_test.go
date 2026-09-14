package iot

import (
	"encoding/json"
	"strings"
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
	// The core vocabulary must stay registered; the set is the
	// things:control vocabulary gate.
	for _, cmd := range []string{
		CmdDoorOpen, CmdDoorClose,
		CmdDeviceOn, CmdDeviceOff, CmdVolumeSet, CmdStateQuery,
		CmdSceneStartClass, CmdSceneEndClass,
	} {
		if !KnownCommandType(cmd) {
			t.Fatalf("command %s missing from vocabulary", cmd)
		}
	}
	if KnownCommandType("format_disk") {
		t.Fatal("unexpected command in vocabulary")
	}
}

func TestValidCommandType(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"door_open", true},
		{"a", true},
		{"door1_open2", true},
		{strings.Repeat("a", 64), true},  // grammar bound
		{strings.Repeat("a", 65), false}, // over the bound
		{"", false},
		{"Door_Open", false}, // uppercase
		{"door-open", false}, // hyphen is not part of the grammar
		{"door open", false}, // space
		{"_door", false},     // must start with a letter
		{"1door", false},     // must start with a letter
	}
	for _, tc := range cases {
		if got := validCommandType(tc.in); got != tc.want {
			t.Fatalf("validCommandType(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRegisterCommandTypes(t *testing.T) {
	// A registered deployment type becomes known immediately.
	RegisterCommandTypes("curtain_open")
	if !KnownCommandType("curtain_open") {
		t.Fatal("registered type not known")
	}

	// Duplicates — against the core set and against an earlier registration —
	// are wiring errors, not silently-ignored no-ops.
	assertRegisterPanic(t, CmdDoorOpen)
	assertRegisterPanic(t, "curtain_open")

	// Malformed names are wiring errors too.
	for _, bad := range []string{"", "Door_Open", "door-open", "door open", "_door"} {
		assertRegisterPanic(t, bad)
	}
}

func assertRegisterPanic(t *testing.T, cmdType string) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("RegisterCommandTypes(%q) should have panicked", cmdType)
		}
	}()
	RegisterCommandTypes(cmdType)
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
