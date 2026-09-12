package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"ocm-backend/internal/iot"
)

// publishTimeout bounds the QoS1 PUBACK wait. Publishing happens inside the
// audited REST request, so a stalled broker must fail the command (it is
// marked failed, never left queued) instead of hanging the console.
const publishTimeout = 5 * time.Second

// Publisher writes backend-originated messages (currently: commands) to the
// data plane over the consumer's connection. It implements
// iot.CommandPublisher.
type Publisher struct {
	cfg    iot.Config
	client pahomqtt.Client
}

// PublishCommand sends one command frame on the device's cmd topic and flips
// the registry row queued → delivered once the broker accepts the publish.
// ctx currently cannot cancel an in-flight paho publish (the library's token
// API has no context support); the 5s PUBACK timeout is the effective bound.
func (p *Publisher) PublishCommand(ctx context.Context, d iot.Device, cmd iot.DeviceCommand) error {
	if p.client == nil {
		return errors.New("mqtt client not connected")
	}
	frame := CommandFrame{
		V:         PayloadVersion,
		CommandID: cmd.CommandID,
		Type:      cmd.Type,
		Payload:   cmd.Payload,
		IssuedAt:  cmd.CreatedAt.UnixMilli(),
		ExpiresAt: cmd.ExpiresAt.UnixMilli(),
	}
	b, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal command frame: %w", err)
	}
	topic := CommandTopic(d.Site, d.SourceID, d.ExternalID)
	token := p.client.Publish(topic, 1, false, b)
	if !token.WaitTimeout(publishTimeout) {
		return fmt.Errorf("mqtt publish timeout on %s", topic)
	}
	return token.Error()
}
