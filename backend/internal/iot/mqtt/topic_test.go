package mqtt

import (
	"errors"
	"testing"
)

func TestParseDataTopic(t *testing.T) {
	cases := []struct {
		name    string
		topic   string
		site    string
		source  string
		device  string
		channel string
		wantErr bool
	}{
		{name: "state", topic: "iot/main/gw01/C301-door/state", site: "main", source: "gw01", device: "C301-door", channel: "state"},
		{name: "event", topic: "iot/campusA/ctrl-1/aircon/event", site: "campusA", source: "ctrl-1", device: "aircon", channel: "event"},
		{name: "ack", topic: "iot/main/gw01/C301-door/ack", site: "main", source: "gw01", device: "C301-door", channel: "ack"},
		{name: "cmd (recognized, consumed as loopback)", topic: "iot/main/gw01/C301-door/cmd", site: "main", source: "gw01", device: "C301-door", channel: "cmd"},
		{name: "wrong root", topic: "things/main/gw01/d/state", wantErr: true},
		{name: "too few segments", topic: "iot/main/gw01/state", wantErr: true},
		{name: "too many segments", topic: "iot/main/gw01/d/extra/state", wantErr: true},
		{name: "meta namespace is not a data topic", topic: "iot/_meta/gw01/offline", wantErr: true},
		{name: "empty segment", topic: "iot/main//d/state", wantErr: true},
		{name: "wildcard segment", topic: "iot/main/+/d/state", wantErr: true},
		{name: "broker-reserved prefix", topic: "iot/$sys/gw01/d/state", wantErr: true},
		{name: "invalid charset", topic: "iot/main/gw 1/d/state", wantErr: true},
		{name: "unknown channel", topic: "iot/main/gw01/d/telemetry", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site, source, device, channel, err := ParseDataTopic(tc.topic)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDataTopic(%q) = %v, %v, %v, %v, want error", tc.topic, site, source, device, channel)
				}
				if !errors.Is(err, ErrBadTopic) {
					t.Fatalf("error should wrap ErrBadTopic, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDataTopic(%q) unexpected error: %v", tc.topic, err)
			}
			if site != tc.site || source != tc.source || device != tc.device || channel != tc.channel {
				t.Fatalf("ParseDataTopic(%q) = %q/%q/%q/%q, want %q/%q/%q/%q",
					tc.topic, site, source, device, channel, tc.site, tc.source, tc.device, tc.channel)
			}
		})
	}
}

func TestParseMetaTopic(t *testing.T) {
	source, channel, err := ParseMetaTopic("iot/_meta/gw01/offline")
	if err != nil || source != "gw01" || channel != "offline" {
		t.Fatalf("ParseMetaTopic = %q, %q, %v", source, channel, err)
	}
	for _, topic := range []string{
		"iot/main/gw01/offline",   // data-shaped, not meta
		"iot/_meta/gw01",          // missing channel
		"iot/_meta/gw01/a/b",      // too many segments
		"iot/_meta/gw 01/offline", // bad charset
	} {
		if _, _, err := ParseMetaTopic(topic); err == nil {
			t.Fatalf("ParseMetaTopic(%q) should fail", topic)
		}
	}
}

func TestCommandTopic(t *testing.T) {
	got := CommandTopic("main", "gw01", "C301-door")
	if got != "iot/main/gw01/C301-door/cmd" {
		t.Fatalf("CommandTopic = %q", got)
	}
	if _, _, _, _, err := ParseDataTopic(got); err != nil {
		t.Fatalf("CommandTopic output must round-trip ParseDataTopic: %v", err)
	}
}

func TestIsMetaTopic(t *testing.T) {
	if !IsMetaTopic("iot/_meta/gw01/offline") {
		t.Fatal("meta topic not recognized")
	}
	if IsMetaTopic("iot/main/gw01/d/state") {
		t.Fatal("data topic misclassified as meta")
	}
}
