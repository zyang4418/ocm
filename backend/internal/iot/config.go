package iot

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults for the IoT data plane. They can be overridden per deployment via
// the IOT_* environment variables (see .env.example and the deploy guide).
const (
	// DefaultSiteID is the {site} segment of the topic namespace when the
	// deployment does not set IOT_SITE_ID. It exists so several schools can
	// share one broker; single-broker-per-school deployments never see it.
	DefaultSiteID = "main"
	// DefaultEventRetentionDays bounds iot_events / iot_commands rows.
	DefaultEventRetentionDays = 90
	// DefaultOnlineTTL is how long an approved device stays "online" after
	// its last state message before the stale sweep flips it offline. Sources
	// that vanish politely flip it earlier via their will message, so this is
	// only the safety net — but it must exceed the slowest legitimate report
	// period of any attached source, or healthy devices flap offline/online
	// every cycle (a classroom controller pushing telemetry every ~5m10s was
	// misjudged by the 5m default on real hardware).
	DefaultOnlineTTL = 15 * time.Minute
)

// Config is the IoT data-plane configuration, read once at startup.
type Config struct {
	// MQTTURL is the broker endpoint (e.g. tcp://mosquitto:1883). Empty means
	// the data plane is disabled: routes still register, the consumer and
	// publisher do not start, and command issuing answers 503.
	MQTTURL      string
	MQTTUsername string
	MQTTPassword string

	SiteID             string
	EventRetentionDays int
	OnlineTTL          time.Duration
}

// Enabled reports whether the MQTT data plane is configured.
func (c Config) Enabled() bool { return c.MQTTURL != "" }

// ConfigFromEnv reads the IOT_* environment variables. Missing values fall
// back to the defaults; an unset IOT_MQTT_URL disables the data plane
// (fail-soft by design: the registry module must not take the platform down
// when a school runs OCM without IoT infrastructure).
func ConfigFromEnv() Config {
	return Config{
		MQTTURL:            strings.TrimSpace(os.Getenv("IOT_MQTT_URL")),
		MQTTUsername:       os.Getenv("IOT_MQTT_USERNAME"),
		MQTTPassword:       os.Getenv("IOT_MQTT_PASSWORD"),
		SiteID:             envOrDefault("IOT_SITE_ID", DefaultSiteID),
		EventRetentionDays: envIntOrDefault("IOT_EVENT_RETENTION_DAYS", DefaultEventRetentionDays),
		OnlineTTL:          envSecondsOrDefault("IOT_ONLINE_TTL_SECONDS", DefaultOnlineTTL),
	}
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envSecondsOrDefault(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return fallback
}
