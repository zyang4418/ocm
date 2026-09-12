package mqttc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

type logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Options configures one device-side connection (one source = one MQTT
// credential, spec §8).
type Options struct {
	BrokerURL  string // e.g. tcp://mosquitto:1883 (ssl:// for TLS)
	Username   string
	Password   string
	ClientID   string // default "iotc-<sourceId>"
	Site       string // default "main"
	SourceID   string // required; the topic segment this source publishes under
	BufferPath string // event/ack FIFO file; empty = memory only
	BufferMax  int    // FIFO capacity, default 10000 (oldest dropped)
	Log        logger // default slog.Default()
}

// Command is a parsed downlink command delivered to a CommandHandler.
type Command struct {
	DeviceID  string
	CommandID string
	Type      string
	Payload   json.RawMessage
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Outcome is what a driver reports back for one command (spec §5).
type Outcome struct {
	Status string // AckStatusAcked | AckStatusFailed
	Detail string
}

// CommandHandler executes one command. It runs on its own goroutine; slow
// handlers never stall the MQTT pipeline. SAFETY (spec §5): the SDK has
// already dropped (and acked failed) commands whose expires_at passed — the
// handler only ever sees live commands.
type CommandHandler func(ctx context.Context, cmd Command) Outcome

// publishTimeout bounds the QoS1 PUBACK wait per frame.
const publishTimeout = 5 * time.Second

// Client is one device-side connection. It is safe for concurrent use.
type Client struct {
	opts       Options
	log        logger
	client     pahomqtt.Client
	dataPrefix string // iot/{site}/{sourceId}
	buf        *queue

	mu     sync.Mutex
	latest map[string]latestState // deviceID → last state, replayed on reconnect

	cmdMu      sync.Mutex
	cmdHandler CommandHandler
}

type latestState struct {
	category string
	payload  []byte
}

// Connect dials the broker and returns a ready client. The initial connect
// honors ctx (a school network may be down for hours — fail fast and let the
// caller decide); after the first success, reconnection is automatic.
func Connect(ctx context.Context, opts Options) (*Client, error) {
	if opts.BrokerURL == "" {
		return nil, fmt.Errorf("mqttc: broker url required")
	}
	if !ValidSegment(opts.SourceID) {
		return nil, ErrBadSourceID
	}
	if opts.Site == "" {
		opts.Site = "main"
	}
	if opts.ClientID == "" {
		opts.ClientID = "iotc-" + opts.SourceID
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	c := &Client{
		opts:       opts,
		log:        opts.Log,
		dataPrefix: TopicRoot + "/" + opts.Site + "/" + opts.SourceID,
		latest:     make(map[string]latestState),
	}

	q, err := openQueue(opts.BufferPath, opts.BufferMax, c.log)
	if err != nil {
		return nil, err
	}
	c.buf = q
	if q.path != "" {
		ensureDir(q.path)
	}

	clientOpts := pahomqtt.NewClientOptions().
		AddBroker(opts.BrokerURL).
		SetClientID(opts.ClientID).
		SetUsername(opts.Username).
		SetPassword(opts.Password).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5*time.Second).
		SetKeepAlive(30*time.Second).
		// Will: the backend marks every device of this source offline when it
		// fires (spec §6) — a site-level outage signal, not per-device.
		SetWill(metaTopic(opts.SourceID, MetaChannelOffline), string(mustWillPayload()), 1, false).
		SetOnConnectHandler(c.onConnect).
		SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
			c.log.Warn("mqttc: connection lost", "err", err)
		})
	c.client = pahomqtt.NewClient(clientOpts)

	t := c.client.Connect()
	select {
	case <-t.Done():
		if err := t.Error(); err != nil {
			return nil, fmt.Errorf("mqttc: connect %s: %w", opts.BrokerURL, err)
		}
	case <-ctx.Done():
		c.client.Disconnect(0)
		return nil, fmt.Errorf("mqttc: connect canceled: %w", ctx.Err())
	}
	c.log.Info("mqttc: connected", "broker", opts.BrokerURL, "source", opts.SourceID, "site", opts.Site)
	return c, nil
}

func mustWillPayload() []byte {
	// The backend routes by topic, not payload; the frame still validates as
	// a v1 envelope so intermediaries see well-formed JSON.
	return []byte(`{"v":1,"note":"source will: offline"}`)
}

// onConnect replays state, restores the command subscription and drains the
// event/ack buffer. paho fires it on the first connect and on every
// reconnect.
func (c *Client) onConnect(_ pahomqtt.Client) {
	c.mu.Lock()
	snapshot := make(map[string]latestState, len(c.latest))
	for id, st := range c.latest {
		snapshot[id] = st
	}
	c.mu.Unlock()
	for deviceID, st := range snapshot {
		topic := dataTopic(c.opts.Site, c.opts.SourceID, deviceID, ChannelState)
		t := c.client.Publish(topic, 1, true, st.payload)
		if !t.WaitTimeout(publishTimeout) || t.Error() != nil {
			c.log.Warn("mqttc: state replay failed", "device", deviceID, "err", t.Error())
		}
	}

	c.cmdMu.Lock()
	handler := c.cmdHandler
	c.cmdMu.Unlock()
	if handler != nil {
		if t := c.client.Subscribe(c.dataPrefix+"/+/cmd", 1, c.onCommand); !t.WaitTimeout(publishTimeout) || t.Error() != nil {
			c.log.Error("mqttc: command subscribe failed", "err", t.Error())
		}
	}

	go c.drain()
}

// drain flushes the event/ack FIFO; used on (re)connect.
func (c *Client) drain() {
	if _, err := c.buf.drain(func(e queueEntry) error {
		t := c.client.Publish(e.Topic, 1, e.Retain, e.Payload)
		if !t.WaitTimeout(publishTimeout) {
			return fmt.Errorf("publish timeout on %s", e.Topic)
		}
		return t.Error()
	}); err != nil {
		c.log.Warn("mqttc: buffer drain stopped (will retry on reconnect)", "pending", c.buf.len(), "err", err)
		return
	}
	if n := c.buf.len(); n == 0 {
		c.log.Info("mqttc: buffer drained")
	}
}

// PublishState reports a device's latest attribute set (retained, spec §3).
// The SDK keeps the frame in memory and republishes it automatically on
// reconnect, so a failed transport does not lose the state — the returned
// error is informational, not a data-loss signal.
func (c *Client) PublishState(ctx context.Context, deviceID string, at time.Time, category string, attrs map[string]any) error {
	if !ValidSegment(deviceID) {
		return fmt.Errorf("mqttc: invalid deviceId %q", deviceID)
	}
	payload, err := marshalPayload(StatePayload{
		V:        PayloadVersion,
		At:       millis(at),
		Category: category,
		Attrs:    mustJSON(attrs),
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.latest[deviceID] = latestState{category: category, payload: payload}
	c.mu.Unlock()
	return c.publish(dataTopic(c.opts.Site, c.opts.SourceID, deviceID, ChannelState), payload, true)
}

// PublishEvent reports an async device event. On transport failure the frame
// is queued persistently and re-sent on reconnect (spec §7); the returned
// error is nil in that case — it is only non-nil when the event could not be
// encoded or buffered at all (i.e. real data loss).
func (c *Client) PublishEvent(ctx context.Context, deviceID string, at time.Time, eventType string, data map[string]any) error {
	if !ValidSegment(deviceID) {
		return fmt.Errorf("mqttc: invalid deviceId %q", deviceID)
	}
	if !validEventName(eventType) {
		return fmt.Errorf("mqttc: invalid event type %q", eventType)
	}
	payload, err := marshalPayload(EventPayload{
		V:    PayloadVersion,
		At:   millis(at),
		Type: eventType,
		Data: mustJSON(data),
	})
	if err != nil {
		return err
	}
	topic := dataTopic(c.opts.Site, c.opts.SourceID, deviceID, ChannelEvent)
	if err := c.publish(topic, payload, false); err != nil {
		if qerr := c.buf.add(queueEntry{Topic: topic, Payload: payload}); qerr != nil {
			return fmt.Errorf("mqttc: event lost (publish failed: %v; buffer failed: %w)", err, qerr)
		}
		c.log.Warn("mqttc: event buffered (offline)", "device", deviceID, "type", eventType)
		return nil
	}
	return nil
}

// SubscribeCommands registers the command handler and subscribes to this
// source's cmd topics. Call once before or after Connect; the subscription is
// re-established on every reconnect.
func (c *Client) SubscribeCommands(ctx context.Context, handler CommandHandler) error {
	if handler == nil {
		return fmt.Errorf("mqttc: nil command handler")
	}
	c.cmdMu.Lock()
	c.cmdHandler = handler
	c.cmdMu.Unlock()
	t := c.client.Subscribe(c.dataPrefix+"/+/cmd", 1, c.onCommand)
	if !t.WaitTimeout(publishTimeout) || t.Error() != nil {
		return fmt.Errorf("mqttc: subscribe commands: %w", t.Error())
	}
	return nil
}

// onCommand parses a command frame. Expired commands are acked failed and
// never reach the driver (spec §5 safety rule).
func (c *Client) onCommand(_ pahomqtt.Client, m pahomqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("mqttc: command handler panic", "panic", r)
		}
	}()
	// topic: {dataPrefix}/{deviceID}/cmd
	rel := strings.TrimPrefix(m.Topic(), c.dataPrefix+"/")
	deviceID := strings.TrimSuffix(rel, "/"+ChannelCmd)
	if deviceID == "" || !ValidSegment(deviceID) {
		c.log.Warn("mqttc: command on unparseable topic", "topic", m.Topic())
		return
	}
	var f CommandFrame
	if err := json.Unmarshal(m.Payload(), &f); err != nil {
		c.log.Warn("mqttc: malformed command frame", "topic", m.Topic())
		return
	}
	if f.V != PayloadVersion || len(f.CommandID) != 32 {
		c.log.Warn("mqttc: invalid command frame", "topic", m.Topic())
		return
	}
	expiresAt := time.UnixMilli(f.ExpiresAt)
	if time.Now().After(expiresAt) {
		c.log.Warn("mqttc: dropping expired command", "command_id", f.CommandID, "type", f.Type)
		_ = c.reportAck(deviceID, f.CommandID, Outcome{Status: AckStatusFailed, Detail: "expired before execution"})
		return
	}
	cmd := Command{
		DeviceID:  deviceID,
		CommandID: f.CommandID,
		Type:      f.Type,
		Payload:   f.Payload,
		IssuedAt:  time.UnixMilli(f.IssuedAt),
		ExpiresAt: expiresAt,
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.log.Error("mqttc: command handler panic", "command_id", cmd.CommandID, "panic", r)
				_ = c.reportAck(cmd.DeviceID, cmd.CommandID, Outcome{Status: AckStatusFailed, Detail: "driver panic"})
			}
		}()
		c.cmdMu.Lock()
		handler := c.cmdHandler
		c.cmdMu.Unlock()
		out := handler(context.Background(), cmd)
		_ = c.reportAck(cmd.DeviceID, cmd.CommandID, out)
	}()
}

// Ack reports a command outcome (convenience wrapper over the raw ack).
func (c *Client) Ack(ctx context.Context, cmd Command, out Outcome) error {
	return c.reportAck(cmd.DeviceID, cmd.CommandID, out)
}

func (c *Client) reportAck(deviceID, commandID string, out Outcome) error {
	payload, err := marshalPayload(AckPayload{
		V:         PayloadVersion,
		CommandID: commandID,
		Status:    out.Status,
		Detail:    out.Detail,
		At:        millis(time.Now()),
	})
	if err != nil {
		return err
	}
	topic := dataTopic(c.opts.Site, c.opts.SourceID, deviceID, ChannelAck)
	if err := c.publish(topic, payload, false); err != nil {
		if qerr := c.buf.add(queueEntry{Topic: topic, Payload: payload}); qerr != nil {
			return fmt.Errorf("mqttc: ack lost (publish failed: %v; buffer failed: %w)", err, qerr)
		}
	}
	return nil
}

func (c *Client) publish(topic string, payload []byte, retain bool) error {
	t := c.client.Publish(topic, 1, retain, payload)
	if !t.WaitTimeout(publishTimeout) {
		return fmt.Errorf("mqttc: publish timeout on %s", topic)
	}
	return t.Error()
}

// Buffered reports how many frames are waiting in the offline buffer.
func (c *Client) Buffered() int { return c.buf.len() }

// Close unsubscribes and disconnects. The will message fires, telling the
// backend this source went away.
func (c *Client) Close() {
	c.client.Disconnect(250)
	c.log.Info("mqttc: disconnected", "source", c.opts.SourceID)
}

// mustJSON encodes attrs/data maps; nil becomes a JSON null (omitted meaning:
// no attributes). Encode errors are impossible for map[string]any inputs, so
// a panic is the honest signal for a programming error.
func mustJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("mqttc: encode attributes: %w", err))
	}
	return b
}
