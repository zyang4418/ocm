// Package mqtt implements the OCM IoT data plane on top of an MQTT broker:
// the backend-side consumer (device → registry) and publisher (console →
// device), plus the topic grammar and payload contracts both sides share.
//
// The topic namespace is:
//
//	iot/{site}/{sourceId}/{deviceId}/{channel}   device data & commands
//	iot/_meta/{sourceId}/{channel}               source lifecycle (will/heartbeat)
//
// Segments carry stable identifiers only (no schema versions, no classroom
// bindings — those live in the registry). channel is one of state | event |
// cmd | ack; the backend publishes cmd and consumes the other three.
package mqtt

import (
	"errors"
	"fmt"
	"strings"
)

// Topic vocabulary.
const (
	TopicRoot          = "iot"
	MetaSegment        = "_meta"
	ChannelState       = "state"
	ChannelEvent       = "event"
	ChannelCmd         = "cmd"
	ChannelAck         = "ack"
	MetaChannelOffline = "offline" // will-message topic: source vanished
)

var ErrBadTopic = errors.New("malformed iot topic")

// ParseDataTopic splits a device-data topic into its identifier segments.
// channel is validated against the full channel vocabulary (including cmd, so
// the consumer can recognize — and ignore — the loopback of its own
// publishes).
func ParseDataTopic(topic string) (site, sourceID, deviceID, channel string, err error) {
	segs := strings.Split(topic, "/")
	if len(segs) != 5 || segs[0] != TopicRoot || segs[1] == MetaSegment {
		return "", "", "", "", fmt.Errorf("%w: %q", ErrBadTopic, topic)
	}
	site, sourceID, deviceID, channel = segs[1], segs[2], segs[3], segs[4]
	for _, s := range []string{site, sourceID, deviceID} {
		if !validSegment(s) {
			return "", "", "", "", fmt.Errorf("%w: invalid segment %q in %q", ErrBadTopic, s, topic)
		}
	}
	switch channel {
	case ChannelState, ChannelEvent, ChannelCmd, ChannelAck:
	default:
		return "", "", "", "", fmt.Errorf("%w: unknown channel %q", ErrBadTopic, channel)
	}
	return site, sourceID, deviceID, channel, nil
}

// ParseMetaTopic splits a source-lifecycle topic (iot/_meta/{source}/{chan}).
func ParseMetaTopic(topic string) (sourceID, channel string, err error) {
	segs := strings.Split(topic, "/")
	if len(segs) != 4 || segs[0] != TopicRoot || segs[1] != MetaSegment {
		return "", "", fmt.Errorf("%w: %q", ErrBadTopic, topic)
	}
	sourceID, channel = segs[2], segs[3]
	if !validSegment(sourceID) || !validSegment(channel) {
		return "", "", fmt.Errorf("%w: invalid segment in %q", ErrBadTopic, topic)
	}
	return sourceID, channel, nil
}

// CommandTopic is the downlink topic the backend publishes commands to.
func CommandTopic(site, sourceID, deviceID string) string {
	return TopicRoot + "/" + site + "/" + sourceID + "/" + deviceID + "/" + ChannelCmd
}

// IsMetaTopic reports whether topic lives in the _meta namespace.
func IsMetaTopic(topic string) bool {
	return strings.HasPrefix(topic, TopicRoot+"/"+MetaSegment+"/")
}

// validSegment enforces the identifier grammar: non-empty, no MQTT
// wildcards/reserved prefixes, printable ASCII subset. Topics are stable
// identifiers, so the charset is deliberately narrow.
func validSegment(s string) bool {
	if s == "" || len(s) > 128 || s[0] == '$' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.':
		default:
			return false
		}
	}
	return true
}
