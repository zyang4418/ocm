// Package sim implements the virtual classroom fleet: a configurable set of
// classrooms whose devices publish canonical state/event/ack traffic through
// the mqttc SDK. It is the executable sample of iot/spec/spec.md, the
// development stand-in for real hardware, and the e2e test fixture.
package sim

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"ocm-iot/mqttc"
)

// Device categories published by the simulator. Attrs are intentionally
// simple; the console renders unknown attributes as raw JSON either way.
const (
	CatDoorSensor = "door_sensor"
	CatAircon     = "aircon"
	CatProjector  = "projector"
	CatScreen     = "screen"
	CatAmbient    = "ambient"
)

// Options configures the virtual fleet.
type Options struct {
	Classrooms int           // number of classrooms
	Prefix     string        // classroom name prefix, default "C"
	StartIndex int           // first classroom number, default 301
	Interval   time.Duration // state tick per classroom, default 5s
	AlarmP     float64       // per-classroom per-tick alarm probability, default 0.02
	Log        *slog.Logger
}

type classroom struct {
	name        string
	doorOpen    bool
	airconOn    bool
	projectorOn bool
	screenDown  bool
	temp        float64
	humidity    float64
	co2         int
}

func (c *classroom) tick(r *rand.Rand) {
	// Ambient random walk keeps the detail page visibly "alive".
	c.temp += (r.Float64() - 0.5) * 0.4
	c.temp = clamp(c.temp, 16, 32)
	c.humidity += (r.Float64() - 0.5) * 2
	c.humidity = clamp(c.humidity, 30, 80)
	c.co2 += r.Intn(41) - 20
	if c.co2 < 400 {
		c.co2 = 400
	}
	if c.co2 > 1500 {
		c.co2 = 1500
	}
	// Doors occasionally move on their own (people passing through).
	if r.Intn(20) == 0 {
		c.doorOpen = !c.doorOpen
	}
	if !c.doorOpen && r.Intn(60) == 0 {
		c.doorOpen = true
	}
}

func (c *classroom) startClass() {
	c.projectorOn = true
	c.screenDown = true
	c.airconOn = true
}

func (c *classroom) endClass() {
	c.projectorOn = false
	c.screenDown = false
	c.airconOn = false
}

func (c *classroom) attrs(deviceKind string) (string, map[string]any) {
	switch deviceKind {
	case CatDoorSensor:
		return CatDoorSensor, map[string]any{"open": c.doorOpen}
	case CatAircon:
		return CatAircon, map[string]any{"power": c.airconOn, "mode": "cool", "targetTemp": 26}
	case CatProjector:
		return CatProjector, map[string]any{"power": c.projectorOn, "input": "hdmi1"}
	case CatScreen:
		return CatScreen, map[string]any{"position": map[bool]string{true: "down", false: "up"}[c.screenDown]}
	case CatAmbient:
		return CatAmbient, map[string]any{
			"temperature": round1(c.temp),
			"humidity":    round1(c.humidity),
			"co2":         c.co2,
		}
	}
	return "generic", map[string]any{}
}

// Run drives the fleet until ctx is done. It registers one command handler
// for every simulated device and publishes state on every tick.
func Run(ctx context.Context, client *mqttc.Client, opts Options, log *slog.Logger) error {
	if opts.Prefix == "" {
		opts.Prefix = "C"
	}
	if opts.StartIndex == 0 {
		opts.StartIndex = 301
	}
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.AlarmP <= 0 {
		opts.AlarmP = 0.02
	}
	if log == nil {
		log = slog.Default()
	}

	classrooms := make([]*classroom, 0, opts.Classrooms)
	for i := 0; i < opts.Classrooms; i++ {
		classrooms = append(classrooms, &classroom{name: fmt.Sprintf("%s%d", opts.Prefix, opts.StartIndex+i)})
	}

	// One handler for the whole fleet; the device kind is the deviceId
	// suffix (e.g. C301-door → door).
	err := client.SubscribeCommands(ctx, func(_ context.Context, cmd mqttc.Command) mqttc.Outcome {
		// deviceId shape is "<classroom>-<kind>" (e.g. C301-door); the kind
		// decides which switch the command hits.
		kind := deviceKind(cmd.DeviceID)
		name := strings.TrimSuffix(cmd.DeviceID, "-"+kind)
		cls := findClassroom(classrooms, name)
		if cls == nil {
			return mqttc.Outcome{Status: mqttc.AckStatusFailed, Detail: "unknown device"}
		}
		switch cmd.Type {
		case "door_open":
			cls.doorOpen = true
			return mqttc.Outcome{Status: mqttc.AckStatusAcked}
		case "door_close":
			cls.doorOpen = false
			return mqttc.Outcome{Status: mqttc.AckStatusAcked}
		case "scene_start_class":
			cls.startClass()
			return mqttc.Outcome{Status: mqttc.AckStatusAcked}
		case "scene_end_class":
			cls.endClass()
			return mqttc.Outcome{Status: mqttc.AckStatusAcked}
		default:
			return mqttc.Outcome{Status: mqttc.AckStatusFailed, Detail: "unsupported command type"}
		}
	})
	if err != nil {
		return fmt.Errorf("sim: subscribe commands: %w", err)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, cls := range classrooms {
				cls.tick(r)
				for _, kind := range []string{CatDoorSensor, CatAircon, CatProjector, CatScreen, CatAmbient} {
					category, attrs := cls.attrs(kind)
					if err := client.PublishState(ctx, deviceIdOf(cls.name, kind), time.Now(), category, attrs); err != nil {
						log.Warn("sim: publish state failed", "device", deviceIdOf(cls.name, kind), "err", err)
					}
				}
				// Occasional device alarm so the event pipeline has traffic.
				if r.Float64() < opts.AlarmP {
					evtType, data := alarm(r, cls)
					if err := client.PublishEvent(ctx, deviceIdOf(cls.name, CatAmbient), time.Now(), evtType, data); err != nil {
						log.Warn("sim: publish event failed", "classroom", cls.name, "err", err)
					} else {
						log.Info("sim: alarm", "classroom", cls.name, "type", evtType)
					}
				}
			}
		}
	}
}

// deviceKind extracts the trailing category from a deviceId like C301-door.
func deviceKind(deviceID string) string {
	if i := strings.LastIndexByte(deviceID, '-'); i >= 0 {
		return deviceID[i+1:]
	}
	return ""
}

func deviceIdOf(classroom, kind string) string { return classroom + "-" + kind }

func findClassroom(cls []*classroom, name string) *classroom {
	for _, c := range cls {
		if c.name == name {
			return c
		}
	}
	return nil
}

func alarm(r *rand.Rand, c *classroom) (string, map[string]any) {
	if c.co2 > 1000 {
		return "ambient.co2_high", map[string]any{"co2": c.co2}
	}
	return "aircon.filter_due", map[string]any{"hours": 500 + r.Intn(200)}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
