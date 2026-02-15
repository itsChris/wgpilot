package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultRingSize is the default number of entries kept in the ring buffer.
const DefaultRingSize = 500

// LogEntry represents a single log entry stored in the ring buffer.
type LogEntry struct {
	Timestamp time.Time
	Level     slog.Level
	Message   string
	Attrs     map[string]any
}

// RingBuffer is a thread-safe circular buffer for log entries.
type RingBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	size    int
	pos     int
	count   int
}

// NewRingBuffer creates a ring buffer that holds the last size entries.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		entries: make([]LogEntry, size),
		size:    size,
	}
}

// Write adds a log entry to the ring buffer.
func (rb *RingBuffer) Write(entry LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.entries[rb.pos%rb.size] = entry
	rb.pos++
	if rb.count < rb.size {
		rb.count++
	}
}

// Recent returns the last n entries in chronological order.
// If n exceeds the number of stored entries, all entries are returned.
func (rb *RingBuffer) Recent(n int) []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.count {
		n = rb.count
	}
	if n == 0 {
		return nil
	}

	result := make([]LogEntry, n)
	start := rb.pos - n
	for i := 0; i < n; i++ {
		idx := (start + i) % rb.size
		if idx < 0 {
			idx += rb.size
		}
		result[i] = rb.entries[idx]
	}
	return result
}

// Len returns the number of entries currently stored.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// ringHandler is a slog.Handler that writes to a primary handler
// and captures WARN+ entries in a ring buffer.
type ringHandler struct {
	primary slog.Handler
	ring    *RingBuffer
	level   slog.Level
	attrs   []slog.Attr
	groups  []string
}

func (h *ringHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.primary.Enabled(ctx, level)
}

func (h *ringHandler) Handle(ctx context.Context, r slog.Record) error {
	// Capture WARN and ERROR entries in the ring buffer.
	if r.Level >= slog.LevelWarn {
		attrs := make(map[string]any)
		for _, a := range h.attrs {
			attrs[a.Key] = a.Value.Any()
		}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.Any()
			return true
		})
		h.ring.Write(LogEntry{
			Timestamp: r.Time,
			Level:     r.Level,
			Message:   r.Message,
			Attrs:     attrs,
		})
	}

	return h.primary.Handle(ctx, r)
}

func (h *ringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ringHandler{
		primary: h.primary.WithAttrs(attrs),
		ring:    h.ring,
		level:   h.level,
		attrs:   append(h.attrs, attrs...),
		groups:  h.groups,
	}
}

func (h *ringHandler) WithGroup(name string) slog.Handler {
	return &ringHandler{
		primary: h.primary.WithGroup(name),
		ring:    h.ring,
		level:   h.level,
		attrs:   h.attrs,
		groups:  append(h.groups, name),
	}
}
