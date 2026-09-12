//go:build integration

// Data-plane e2e for OCM IoT: a real broker + a real MySQL, the contract
// between backend/internal/iot and the device-side SDK exercised end to end
// (a fake device here plays the role the ocm-iot simulator plays in demos —
// the backend module cannot import ocm-iot, see WP-2 notes).
//
// Run locally against a SCRATCH database:
//
//	IOT_TEST_MQTT_URL=tcp://localhost:1883 \
//	IOT_TEST_MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/iot_e2e?parseTime=true' \
//	go test -tags=integration ./internal/iot/...
//
// The test DROPS the iot_* tables in the target database and recreates them.
// In CI the target is the dedicated docker-job database — never point this at
// a database you care about.
package iot_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/iot"
	"ocm-backend/internal/iot/mqtt"
)

const (
	e2eSite     = "e2e"
	e2eSource   = "it-e2e"
	e2eDevice   = "dev1"
	stateTopic  = "iot/e2e/it-e2e/dev1/state"
	cmdFilter   = "iot/e2e/it-e2e/+/cmd"
	willTopic   = "iot/_meta/it-e2e/offline"
	ackTopicFmt = "iot/e2e/it-e2e/dev1/ack"
)

func TestIoTDataPlaneE2E(t *testing.T) {
	brokerURL := envOr(t, "IOT_TEST_MQTT_URL")
	dsn := envOr(t, "IOT_TEST_MYSQL_DSN")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(4)
	for _, tbl := range []string{"iot_commands", "iot_events", "iot_devices"} {
		if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS `+tbl); err != nil {
			t.Fatalf("drop %s: %v", tbl, err)
		}
	}

	store := iot.NewStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	hub := iot.NewHub()
	cfg := iot.Config{MQTTURL: brokerURL, SiteID: e2eSite, OnlineTTL: 5 * time.Minute, EventRetentionDays: 90}
	consumer := mqtt.NewConsumer(ctx, store, hub, cfg)
	publisher := consumer.Publisher()
	go consumer.Run()

	// --- fake device: subscribes cmd, publishes state/event/ack ----------
	fake := pahomqtt.NewClient(pahomqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("it-e2e-device").
		SetCleanSession(true))
	if tok := fake.Connect(); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		t.Fatalf("fake device connect: %v", tok.Error())
	}
	defer fake.Disconnect(250)

	cmdFrames := make(chan mqtt.CommandFrame, 8)
	if tok := fake.Subscribe(cmdFilter, 1, func(_ pahomqtt.Client, m pahomqtt.Message) {
		var f mqtt.CommandFrame
		if err := json.Unmarshal(m.Payload(), &f); err != nil {
			t.Errorf("unmarshal command frame: %v", err)
			return
		}
		cmdFrames <- f
	}); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		t.Fatalf("fake subscribe cmd: %v", tok.Error())
	}

	now := time.Now().UnixMilli()
	state := fmt.Sprintf(`{"v":1,"at":%d,"category":"door_sensor","attrs":{"open":false}}`, now)
	publish(t, fake, stateTopic, state, true)

	// 1) First state report auto-creates a pending device with the declared
	// category and the reported attributes.
	var dev iot.Device
	waitFor(t, "pending device to appear", func() bool {
		list, _, err := store.PageDevices(ctx, iot.DeviceFilter{SourceID: e2eSource}, "", dbutil.Pagination{})
		if err != nil || len(list) != 1 {
			return false
		}
		dev = list[0]
		return dev.Status == iot.StatusPending && dev.Category == "door_sensor"
	})
	var attrs map[string]any
	if err := json.Unmarshal(dev.State, &attrs); err != nil || attrs["open"] != false {
		t.Fatalf("state attrs not stored: %s (%v)", dev.State, err)
	}

	// 2) Approve binds the classroom; a fresh last_seen makes it online.
	dev, err = store.ApproveDevice(ctx, dev.ID, iot.ApproveInput{Name: "E2E 门磁", ClassroomID: 1}, cfg.OnlineTTL)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if dev.Status != iot.StatusOnline || dev.ClassroomID == nil || *dev.ClassroomID != 1 {
		t.Fatalf("approved device wrong: status=%s classroom=%v", dev.Status, dev.ClassroomID)
	}

	// 3) Command round trip: publish → device receives with expires_at →
	// ack flips the row to acked.
	cmd, err := store.CreateCommand(ctx, dev.ID, iot.CmdDoorOpen, nil, "e2e-runner", iot.ClampCommandTTL(30))
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	publishRetry(t, func() error { return publisher.PublishCommand(ctx, dev, cmd) })
	select {
	case f := <-cmdFrames:
		if f.CommandID != cmd.CommandID || f.Type != iot.CmdDoorOpen {
			t.Fatalf("device got wrong frame: %+v", f)
		}
		if f.ExpiresAt <= f.IssuedAt {
			t.Fatalf("frame expires_at not after issued_at: %+v", f)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("device never received the command")
	}
	ack := fmt.Sprintf(`{"v":1,"command_id":%q,"status":"acked","at":%d}`, cmd.CommandID, time.Now().UnixMilli())
	publish(t, fake, ackTopicFmt, ack, false)
	waitFor(t, "command acked", func() bool {
		c, err := store.GetCommandByID(ctx, cmd.CommandID)
		return err == nil && c.Status == iot.CmdStatusAcked
	})

	// 4) Expiry is final: an expired command settles as expired and a late
	// ack can never resurrect it.
	late, err := store.CreateCommand(ctx, dev.ID, iot.CmdDoorClose, nil, "e2e-runner", iot.ClampCommandTTL(1))
	if err != nil {
		t.Fatalf("create late command: %v", err)
	}
	time.Sleep(1500 * time.Millisecond) // let its deadline pass
	if n, err := store.ExpireCommands(ctx); err != nil || n == 0 {
		t.Fatalf("expire commands: n=%d err=%v", n, err)
	}
	if ok, err := store.ApplyAck(ctx, late.CommandID, "acked", ""); err != nil || ok {
		t.Fatalf("late ack must be dropped: ok=%v err=%v", ok, err)
	}
	if c, err := store.GetCommandByID(ctx, late.CommandID); err != nil || c.Status != iot.CmdStatusExpired {
		t.Fatalf("late command status=%s err=%v, want expired", c.Status, err)
	}

	// 5) Event dedup: two identical deliveries (QoS1 at-least-once) store
	// exactly one row.
	evtAt := time.Now().UnixMilli()
	evt := fmt.Sprintf(`{"v":1,"at":%d,"type":"door.forced","data":{"code":"CHAN_0001"}}`, evtAt)
	publish(t, fake, stateToEventTopic(), evt, false)
	time.Sleep(300 * time.Millisecond) // separate the redelivery in broker order
	publish(t, fake, stateToEventTopic(), evt, false)
	waitFor(t, "event stored", func() bool {
		events, total, err := store.PageEvents(ctx, dev.ID, dbutil.Pagination{})
		if err != nil {
			return false
		}
		return total == 1 && len(events) == 1 && events[0].Type == "door.forced"
	})

	// 6) Source will-message marks the whole source offline.
	publish(t, fake, willTopic, `{"v":1}`, false)
	waitFor(t, "source offline", func() bool {
		list, _, err := store.PageDevices(ctx, iot.DeviceFilter{SourceID: e2eSource}, "", dbutil.Pagination{})
		return err == nil && len(list) == 1 && list[0].Status == iot.StatusOffline
	})

	cancel()
}

func stateToEventTopic() string {
	return "iot/e2e/it-e2e/dev1/event"
}

func envOr(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s not set", key)
	}
	return v
}

func publish(t *testing.T, c pahomqtt.Client, topic, payload string, retain bool) {
	t.Helper()
	if tok := c.Publish(topic, 1, retain, []byte(payload)); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		t.Fatalf("publish %s: %v", topic, tok.Error())
	}
}

// publishRetry tolerates the consumer still connecting (ConnectRetry is
// asynchronous) when the first commands are sent.
func publishRetry(t *testing.T, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := fn()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("publish never succeeded: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}
