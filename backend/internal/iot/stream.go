package iot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// streamEvent is one SSE frame pushed from the data plane to console
// subscribers. Names: device.created / device.updated / event.created /
// command.updated / source.offline.
type streamEvent struct {
	Name string
	Data any
}

// Hub is the in-process fan-out between the MQTT consumer and the SSE
// endpoint. The backend is a single process (repo convention), so no
// cross-instance bus is needed. Broadcast drops events for slow subscribers —
// the stream is a live enhancement, the registry DB stays the source of
// truth, and a dropped frame only means the next poll shows the same truth.
type Hub struct {
	mu   sync.Mutex
	subs map[chan streamEvent]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan streamEvent]struct{})}
}

// Subscribe registers a subscriber with a small buffer. The returned cancel
// func unregisters and closes the channel; it is idempotent.
func (h *Hub) Subscribe() (<-chan streamEvent, func()) {
	ch := make(chan streamEvent, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, cancel
}

// Broadcast fans one event out to every subscriber without ever blocking the
// producer (the MQTT callback goroutine): a full subscriber buffer means its
// frames are dropped.
func (h *Hub) Broadcast(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- streamEvent{Name: name, Data: data}:
		default:
		}
	}
}

// pingInterval keeps intermediaries from closing an idle stream. Proxy note:
// the handler sets X-Accel-Buffering: no, matching the AI chat SSE endpoint.
const pingInterval = 15 * time.Second

// stream serves GET /api/iot/stream. Same SSE discipline as the AI chat
// endpoint: everything that can fail before streaming starts answers as JSON;
// once the stream is committed, failures just end it. Gzip passes
// text/event-stream through uncompressed (see middleware.Gzip), so per-frame
// Flush reaches the client immediately.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	ch, unsubscribe := h.hub.Subscribe()
	defer unsubscribe()

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()

	writeFrame := func(name string, data any) bool {
		b, err := json.Marshal(data)
		if err != nil {
			return true // skip the frame, keep the stream alive
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b); err != nil {
			return false
		}
		_ = rc.Flush()
		return true
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !writeFrame(ev.Name, ev.Data) {
				return
			}
		}
	}
}
