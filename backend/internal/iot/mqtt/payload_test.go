package mqtt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatePayloadValidate(t *testing.T) {
	valid := `{"v":1,"at":1730000000000,"category":"door_sensor","attrs":{"open":false}}`
	var p StatePayload
	if err := json.Unmarshal([]byte(valid), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}

	bad := []string{
		`{"v":2,"at":1730000000000,"attrs":{}}`,            // wrong version
		`{"v":1,"at":0,"attrs":{}}`,                        // missing event time
		`{"v":1,"at":1730000000000,"attrs":[1,2]}`,         // attrs not an object
		`{"v":1,"at":1730000000000,"category":"bad name"}`, // category charset
	}
	for _, raw := range bad {
		var p StatePayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("invalid state payload accepted: %s", raw)
		}
	}

	// Empty attrs is legal: a bare report still marks the device seen.
	var bare StatePayload
	if err := json.Unmarshal([]byte(`{"v":1,"at":1730000000000}`), &bare); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := bare.Validate(); err != nil {
		t.Fatalf("bare state payload rejected: %v", err)
	}
}

func TestEventPayloadValidate(t *testing.T) {
	var p EventPayload
	if err := json.Unmarshal([]byte(`{"v":1,"at":1730000000000,"type":"door.forced","data":{"code":"CHAN_0001"}}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	bad := []string{
		`{"v":1,"at":1730000000000}`,                                          // missing type
		`{"v":1,"at":1730000000000,"type":""}`,                                // empty type
		`{"v":1,"at":1730000000000,"type":"has space"}`,                       // space in type
		`{"v":1,"at":1730000000000,"type":"` + strings.Repeat("x", 65) + `"}`, // too long
		`{"v":1,"at":1730000000000,"type":"door","data":[]}`,                  // data not an object
	}
	for _, raw := range bad {
		var p EventPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("invalid event payload accepted: %s", raw)
		}
	}
}

func TestAckPayloadValidate(t *testing.T) {
	var p AckPayload
	if err := json.Unmarshal([]byte(`{"v":1,"command_id":"ab12cd34ab12cd34ab12cd34ab12cd34","status":"acked","at":1730000000000}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid ack rejected: %v", err)
	}

	bad := []string{
		`{"v":1,"command_id":"short","status":"acked"}`,                                // malformed id
		`{"v":1,"command_id":"ab12cd34ab12cd34ab12cd34ab12cd34","status":"delivered"}`, // backend-settled status
		`{"v":1,"command_id":"ab12cd34ab12cd34ab12cd34ab12cd34","status":"nope"}`,
	}
	for _, raw := range bad {
		var p AckPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if err := p.Validate(); err == nil {
			t.Fatalf("invalid ack payload accepted: %s", raw)
		}
	}
}

func TestTimeFromMillis(t *testing.T) {
	// 2025-10-27T22:13:20Z in unix ms; the conversion must land in UTC.
	got := TimeFromMillis(1761608000000)
	if got.UTC() != got {
		t.Fatalf("TimeFromMillis must be UTC, got %v", got)
	}
	if got.UnixMilli() != 1761608000000 {
		t.Fatalf("round-trip mismatch: %d", got.UnixMilli())
	}
}
