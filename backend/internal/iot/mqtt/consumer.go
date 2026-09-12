package mqtt

import (
	"context"
	"encoding/json"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"ocm-backend/internal/iot"
	"ocm-backend/internal/logging"
)

// subscribeFilter is the one subscription the backend needs: everything under
// the root namespace, QoS1, so the broker queues device messages while the
// backend restarts (clean session is disabled below).
const subscribeFilter = TopicRoot + "/#"

// perMessageTimeout bounds each registry write so a slow MySQL can never stall
// the paho message pipeline (its callbacks run in order on one goroutine).
const perMessageTimeout = 5 * time.Second

// Consumer bridges the MQTT data plane into the registry: it subscribes
// iot/# and applies state/event/ack messages. The context is taken at
// construction (not Run) because paho callbacks fire on their own goroutine
// and must never observe a zero context.
type Consumer struct {
	ctx    context.Context
	store  *iot.Store
	hub    *iot.Hub
	cfg    iot.Config
	client pahomqtt.Client
	pub    *Publisher
}

func NewConsumer(ctx context.Context, store *iot.Store, hub *iot.Hub, cfg iot.Config) *Consumer {
	return &Consumer{ctx: ctx, store: store, hub: hub, cfg: cfg}
}

// Publisher returns the command publisher sharing this consumer's connection.
// Must be called before Run connects — main wires it into the handler.
func (c *Consumer) Publisher() *Publisher {
	if c.pub == nil {
		c.pub = &Publisher{cfg: c.cfg}
	}
	return c.pub
}

// Run connects and blocks until ctx is done. CleanSession is disabled so the
// broker queues QoS1 device messages published while the backend is down;
// retained state messages replay the last known device state on every
// subscribe, which is exactly the bootstrap the registry wants.
func (c *Consumer) Run() {
	opts := pahomqtt.NewClientOptions().
		AddBroker(c.cfg.MQTTURL).
		SetClientID("ocm-backend-iot").
		SetUsername(c.cfg.MQTTUsername).
		SetPassword(c.cfg.MQTTPassword).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(c.onConnect).
		SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
			logging.L.Warn("iot mqtt connection lost", "err", err)
		})
	c.client = pahomqtt.NewClient(opts)
	if c.pub != nil {
		c.pub.client = c.client
	}

	// ConnectRetry keeps retrying in the background; the returned token only
	// completes on the first success, which onConnect logs.
	_ = c.client.Connect()

	<-c.ctx.Done()
	if c.client.IsConnectionOpen() {
		_ = c.client.Unsubscribe(subscribeFilter)
	}
	c.client.Disconnect(250)
	logging.L.Info("iot mqtt consumer stopped")
}

// onConnect (re)subscribes — with a persistent session this is redundant on
// reconnect, and deliberately so: re-subscribing is idempotent and guarantees
// the subscription exists even if the broker restarted with a cold store.
func (c *Consumer) onConnect(_ pahomqtt.Client) {
	logging.L.Info("iot mqtt connected", "broker", c.cfg.MQTTURL, "site", c.cfg.SiteID)
	t := c.client.Subscribe(subscribeFilter, 1, c.handleMessage)
	go func() {
		if !t.WaitTimeout(30*time.Second) || t.Error() != nil {
			logging.L.Error("iot mqtt subscribe failed", "err", t.Error())
		}
	}()
}

// handleMessage routes one broker message into the registry. Unknown topics
// are logged and dropped — never a crash; every handler failure is contained.
func (c *Consumer) handleMessage(_ pahomqtt.Client, msg pahomqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			logging.L.Error("iot message handler panic", "topic", msg.Topic(), "panic", r)
		}
	}()
	mctx, cancel := context.WithTimeout(c.ctx, perMessageTimeout)
	defer cancel()

	topic := msg.Topic()
	if IsMetaTopic(topic) {
		c.handleMeta(mctx, topic)
		return
	}
	site, sourceID, deviceID, channel, err := ParseDataTopic(topic)
	if err != nil {
		logging.L.Warn("iot: unroutable topic", "topic", topic)
		return
	}
	switch channel {
	case ChannelCmd:
		// Loopback of our own command publishes; the registry is the source
		// of truth for command state, not the echo.
		return
	case ChannelState:
		c.handleState(mctx, site, sourceID, deviceID, msg.Payload())
	case ChannelEvent:
		c.handleEvent(mctx, site, sourceID, deviceID, msg.Payload())
	case ChannelAck:
		c.handleAck(mctx, msg.Payload())
	}
}

func (c *Consumer) handleState(ctx context.Context, site, sourceID, deviceID string, raw []byte) {
	var p StatePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		logging.L.Warn("iot: malformed state payload", "source", sourceID, "device", deviceID)
		return
	}
	if err := p.Validate(); err != nil {
		logging.L.Warn("iot: invalid state payload", "source", sourceID, "device", deviceID, "err", err)
		return
	}
	d, isNew, err := c.store.UpsertState(ctx, site, sourceID, deviceID, p.Category, p.Attrs, TimeFromMillis(p.At))
	if err != nil {
		logging.L.Error("iot: apply state failed", "source", sourceID, "device", deviceID, "err", err)
		return
	}
	if isNew {
		c.hub.Broadcast("device.created", deviceSummary(d))
	}
	c.hub.Broadcast("device.updated", deviceSummary(d))
}

func (c *Consumer) handleEvent(ctx context.Context, site, sourceID, deviceID string, raw []byte) {
	var p EventPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		logging.L.Warn("iot: malformed event payload", "source", sourceID, "device", deviceID)
		return
	}
	if err := p.Validate(); err != nil {
		logging.L.Warn("iot: invalid event payload", "source", sourceID, "device", deviceID, "err", err)
		return
	}
	// An event from an unknown device must still be recorded — auto-create
	// the pending row, then attach the event.
	d, isNew, err := c.store.EnsureDevice(ctx, site, sourceID, deviceID, "")
	if err != nil {
		logging.L.Error("iot: ensure device for event failed", "source", sourceID, "device", deviceID, "err", err)
		return
	}
	if isNew {
		c.hub.Broadcast("device.created", deviceSummary(d))
	}
	id, err := c.store.InsertEvent(ctx, d.ID, p.Type, p.Data, TimeFromMillis(p.At))
	if err != nil {
		logging.L.Error("iot: insert event failed", "device", d.ExternalID, "type", p.Type, "err", err)
	}
	if id > 0 {
		c.hub.Broadcast("event.created", map[string]any{"deviceId": d.ID, "type": p.Type})
	}
}

func (c *Consumer) handleAck(ctx context.Context, raw []byte) {
	var p AckPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		logging.L.Warn("iot: malformed ack payload")
		return
	}
	if err := p.Validate(); err != nil {
		logging.L.Warn("iot: invalid ack payload", "err", err)
		return
	}
	ok, err := c.store.ApplyAck(ctx, p.CommandID, p.Status, p.Detail)
	if err != nil {
		logging.L.Error("iot: apply ack failed", "command_id", p.CommandID, "err", err)
		return
	}
	if !ok {
		// Unknown id, already settled, or expired — a late ack must never
		// resurrect a settled command.
		logging.L.Warn("iot: ack for dead command", "command_id", p.CommandID, "status", p.Status)
		return
	}
	logging.L.Info("iot command settled", "command_id", p.CommandID, "status", p.Status)
	c.hub.Broadcast("command.updated", map[string]any{"commandId": p.CommandID, "status": p.Status})
}

// handleMeta consumes source-lifecycle messages. The will topic
// iot/_meta/{source}/offline marks the whole source (gateway/controller) and
// thus every online device under it offline — site-level outage signal.
func (c *Consumer) handleMeta(ctx context.Context, topic string) {
	sourceID, channel, err := ParseMetaTopic(topic)
	if err != nil {
		logging.L.Warn("iot: unroutable meta topic", "topic", topic)
		return
	}
	switch channel {
	case MetaChannelOffline:
		n, err := c.store.MarkSourceOffline(ctx, c.cfg.SiteID, sourceID)
		if err != nil {
			logging.L.Error("iot: mark source offline failed", "source", sourceID, "err", err)
			return
		}
		logging.L.Warn("iot: source offline (will message)", "source", sourceID, "devices", n)
		c.hub.Broadcast("source.offline", map[string]any{"sourceId": sourceID, "devices": n})
	default:
		logging.L.Debug("iot: source meta", "source", sourceID, "channel", channel)
	}
}

func deviceSummary(d iot.Device) map[string]any {
	s := map[string]any{
		"id":        d.ID,
		"sourceId":  d.SourceID,
		"deviceId":  d.ExternalID,
		"status":    d.Status,
		"site":      d.Site,
		"updatedAt": d.UpdatedAt.UnixMilli(),
	}
	if d.LastSeenAt != nil {
		s["lastSeenAt"] = d.LastSeenAt.UnixMilli()
	}
	return s
}
