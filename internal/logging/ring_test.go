package logging

import (
	"log/slog"
	"testing"
	"time"
)

func TestRingBuffer_Write_Recent(t *testing.T) {
	rb := NewRingBuffer(5)

	for i := 0; i < 3; i++ {
		rb.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     slog.LevelError,
			Message:   "entry",
		})
	}

	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}

	entries := rb.Recent(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(3)

	for i := 0; i < 7; i++ {
		rb.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     slog.LevelError,
			Message:   time.Now().String(),
			Attrs:     map[string]any{"index": i},
		})
	}

	if rb.Len() != 3 {
		t.Fatalf("expected len 3 (capped at buffer size), got %d", rb.Len())
	}

	entries := rb.Recent(3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Entries should be the last 3 written (index 4, 5, 6).
	for i, e := range entries {
		idx, ok := e.Attrs["index"].(int)
		if !ok {
			t.Fatalf("entry %d: expected int index, got %T", i, e.Attrs["index"])
		}
		expected := 4 + i
		if idx != expected {
			t.Errorf("entry %d: expected index %d, got %d", i, expected, idx)
		}
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer(5)

	entries := rb.Recent(10)
	if entries != nil {
		t.Fatalf("expected nil for empty buffer, got %v", entries)
	}

	if rb.Len() != 0 {
		t.Fatalf("expected len 0, got %d", rb.Len())
	}
}

func TestRingBuffer_RecentLessThanStored(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 8; i++ {
		rb.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     slog.LevelWarn,
			Message:   "msg",
			Attrs:     map[string]any{"index": i},
		})
	}

	entries := rb.Recent(3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should be the last 3: index 5, 6, 7.
	for i, e := range entries {
		idx := e.Attrs["index"].(int)
		expected := 5 + i
		if idx != expected {
			t.Errorf("entry %d: expected index %d, got %d", i, expected, idx)
		}
	}
}
