package mqttc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidSegment(t *testing.T) {
	valid := []string{"gw01", "C301-door", "a.b_c-d", "x"}
	for _, s := range valid {
		if !ValidSegment(s) {
			t.Fatalf("ValidSegment(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "a b", "a/b", "+", "#", "$sys", strings.Repeat("x", 129), "中文"}
	for _, s := range invalid {
		if ValidSegment(s) {
			t.Fatalf("ValidSegment(%q) = true, want false", s)
		}
	}
}

// The wire envelope field names are the cross-module contract with
// backend/internal/iot/mqtt; a rename here silently breaks ingestion.
func TestPayloadWireShapes(t *testing.T) {
	b, _ := marshalPayload(StatePayload{V: PayloadVersion, At: 1730000000000, Category: "door_sensor",
		Attrs: mustJSON(map[string]any{"open": false})})
	var sm map[string]any
	_ = json.Unmarshal(b, &sm)
	for _, k := range []string{"v", "at", "category", "attrs"} {
		if _, ok := sm[k]; !ok {
			t.Fatalf("state payload missing key %q", k)
		}
	}

	b, _ = marshalPayload(EventPayload{V: PayloadVersion, At: 1730000000000, Type: "door.forced",
		Data: mustJSON(map[string]any{"code": "CHAN_0001"})})
	var em map[string]any
	_ = json.Unmarshal(b, &em)
	for _, k := range []string{"v", "at", "type", "data"} {
		if _, ok := em[k]; !ok {
			t.Fatalf("event payload missing key %q", k)
		}
	}

	b, _ = marshalPayload(CommandFrame{V: PayloadVersion, CommandID: "ab12cd34ab12cd34ab12cd34ab12cd34",
		Type: "door_open", IssuedAt: 1730000000000, ExpiresAt: 1730000010000})
	var cm map[string]any
	_ = json.Unmarshal(b, &cm)
	for _, k := range []string{"v", "command_id", "type", "issued_at", "expires_at"} {
		if _, ok := cm[k]; !ok {
			t.Fatalf("command frame missing key %q", k)
		}
	}

	b, _ = marshalPayload(AckPayload{V: PayloadVersion, CommandID: "ab12cd34ab12cd34ab12cd34ab12cd34",
		Status: AckStatusAcked, At: 1730000000000})
	var am map[string]any
	_ = json.Unmarshal(b, &am)
	for _, k := range []string{"v", "command_id", "status"} {
		if _, ok := am[k]; !ok {
			t.Fatalf("ack payload missing key %q", k)
		}
	}
}

func TestTopicBuilders(t *testing.T) {
	if got := dataTopic("main", "gw01", "C301-door", ChannelCmd); got != "iot/main/gw01/C301-door/cmd" {
		t.Fatalf("dataTopic = %q", got)
	}
	if got := TopicRoot + "/" + MetaSegment + "/gw01/offline"; got != "iot/_meta/gw01/offline" {
		t.Fatalf("will topic = %q", got)
	}
}

// The FIFO must survive the process (file reload), shed the oldest frames at
// capacity, and keep unsent entries when a drain fails midway.
func TestQueuePersistDrainShed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buffer.jsonl")

	q, err := openQueue(path, 3, testLogger{})
	if err != nil {
		t.Fatalf("openQueue: %v", err)
	}
	entries := []queueEntry{
		{Topic: "iot/main/g/e1", Payload: []byte(`{"n":1}`)},
		{Topic: "iot/main/g/e2", Payload: []byte(`{"n":2}`)},
		{Topic: "iot/main/g/e3", Payload: []byte(`{"n":3}`)},
		{Topic: "iot/main/g/e4", Payload: []byte(`{"n":4}`)}, // pushes e1 out
	}
	for _, e := range entries {
		if err := q.add(e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if q.len() != 3 {
		t.Fatalf("queue len = %d, want 3 (capacity shed)", q.len())
	}

	// Reload from disk: persistence across "process restarts".
	q2, err := openQueue(path, 3, testLogger{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if q2.len() != 3 {
		t.Fatalf("reloaded len = %d, want 3", q2.len())
	}

	// Drain with a publisher that fails on the second frame: e2 must remain.
	fails := map[string]bool{"iot/main/g/e3": true}
	sent, err := q2.drain(func(e queueEntry) error {
		if fails[e.Topic] {
			return errFakePublish
		}
		return nil
	})
	if err == nil {
		t.Fatal("drain should surface the transport error")
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if q2.len() != 2 {
		t.Fatalf("remaining = %d, want 2", q2.len())
	}

	// Successful drain empties the file.
	if _, err := q2.drain(func(queueEntry) error { return nil }); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if q2.len() != 0 {
		t.Fatalf("queue not empty after drain: %d", q2.len())
	}
	if _, err := os.Stat(path); err == nil {
		b, _ := os.ReadFile(path)
		if len(strings.TrimSpace(string(b))) != 0 {
			t.Fatalf("buffer file not truncated: %q", b)
		}
	}
}

var errFakePublish = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "broker unreachable" }

// Memory-only mode (no buffer path) must behave the same minus persistence.
func TestQueueMemoryOnly(t *testing.T) {
	q, err := openQueue("", 10, testLogger{})
	if err != nil {
		t.Fatalf("openQueue: %v", err)
	}
	if err := q.add(queueEntry{Topic: "t", Payload: []byte(`{}`), Retain: false}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if q.len() != 1 {
		t.Fatalf("len = %d", q.len())
	}
	if _, err := q.drain(func(queueEntry) error { return nil }); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if q.len() != 0 {
		t.Fatalf("not drained: %d", q.len())
	}
}

type testLogger struct{}

func (testLogger) Debug(string, ...any) {}
func (testLogger) Info(string, ...any)  {}
func (testLogger) Warn(string, ...any)  {}
func (testLogger) Error(string, ...any) {}
