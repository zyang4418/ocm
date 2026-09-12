package mqttc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// queue is the persistent FIFO for fire-and-forget frames (events, acks)
// published while the broker is unreachable — spec §7. state frames never
// enter it: only the latest state matters, and that is replayed from memory.
//
// The queue is a JSONL file: one entry per line, appended on add, rewritten
// on trim and on partial drain. Small files, rare operations — correctness
// over cleverness.
type queue struct {
	path string // empty = memory only
	max  int
	log  logger

	mu      sync.Mutex
	entries []queueEntry
}

type queueEntry struct {
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload"`
	Retain  bool            `json:"retain"`
}

// openQueue loads (or creates) the buffer. In-memory mode is used when path
// is empty — events then survive only reconnects, not process restarts.
func openQueue(path string, max int, log logger) (*queue, error) {
	if max <= 0 {
		max = 10000
	}
	q := &queue{path: path, max: max, log: log}
	if path == "" {
		return q, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return q, nil
		}
		return nil, fmt.Errorf("mqttc: open buffer %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e queueEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			// One corrupt line (crash mid-write) must not poison the queue.
			log.Warn("mqttc: dropping corrupt buffer line", "err", err)
			continue
		}
		q.entries = append(q.entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("mqttc: read buffer %s: %w", path, err)
	}
	return q, nil
}

// add enqueues an entry and persists it. When the queue is over capacity the
// oldest entries are dropped (spec: full queue sheds the oldest frames).
func (q *queue) add(e queueEntry) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = append(q.entries, e)
	if len(q.entries) > q.max {
		drop := len(q.entries) - q.max
		q.entries = q.entries[drop:]
		q.log.Warn("mqttc: buffer overflow, dropped oldest frames", "dropped", drop)
	}
	return q.persistLocked()
}

// drain publishes queued entries in order. The first transport failure stops
// the drain; the remaining entries — including the failed one — are kept and
// the file is rewritten so nothing is lost. Returns the frames published.
func (q *queue) drain(publish func(queueEntry) error) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.entries) == 0 {
		return 0, nil
	}
	sent := 0
	for len(q.entries) > 0 {
		e := q.entries[0]
		if err := publish(e); err != nil {
			if err := q.persistLocked(); err != nil {
				q.log.Error("mqttc: buffer rewrite failed", "err", err)
			}
			return sent, err
		}
		q.entries = q.entries[1:]
		sent++
	}
	if err := q.persistLocked(); err != nil {
		q.log.Error("mqttc: buffer rewrite failed", "err", err)
	}
	return sent, nil
}

func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

// persistLocked rewrites the whole file via tmp+rename. Called with mu held.
func (q *queue) persistLocked() error {
	if q.path == "" {
		return nil
	}
	tmp := q.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("mqttc: open buffer tmp: %w", err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, e := range q.entries {
		if err := enc.Encode(e); err != nil {
			_ = f.Close()
			return fmt.Errorf("mqttc: encode buffer entry: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return fmt.Errorf("mqttc: flush buffer: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("mqttc: close buffer tmp: %w", err)
	}
	if err := os.Rename(tmp, q.path); err != nil {
		return fmt.Errorf("mqttc: swap buffer: %w", err)
	}
	return nil
}

// ensureDir creates the parent directory of a buffer path (best effort).
func ensureDir(path string) {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
}
