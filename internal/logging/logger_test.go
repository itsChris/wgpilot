package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestNew_DevMode(t *testing.T) {
	logger := New(Config{
		Level:   slog.LevelDebug,
		DevMode: true,
	})
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug level to be enabled in dev mode")
	}
}

func TestNew_ProductionMode(t *testing.T) {
	logger := New(Config{
		Level:   slog.LevelInfo,
		DevMode: false,
	})
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug level to be disabled in production mode")
	}
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected info level to be enabled in production mode")
	}
}

func TestNewWithRing(t *testing.T) {
	ring := NewRingBuffer(10)
	logger := NewWithRing(Config{
		Level:   slog.LevelWarn,
		DevMode: false,
	}, ring)

	logger.Warn("test warning", "key", "value")
	logger.Error("test error", "key2", "value2")
	logger.Info("should not appear in ring")

	entries := ring.Recent(10)
	if len(entries) != 2 {
		t.Fatalf("expected 2 ring entries, got %d", len(entries))
	}
	if entries[0].Message != "test warning" {
		t.Errorf("expected first entry message %q, got %q", "test warning", entries[0].Message)
	}
	if entries[1].Message != "test error" {
		t.Errorf("expected second entry message %q, got %q", "test error", entries[1].Message)
	}
}
