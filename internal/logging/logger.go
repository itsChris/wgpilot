package logging

import (
	"log/slog"
	"os"
)

// Config controls logger behavior.
type Config struct {
	Level     slog.Level
	DevMode   bool
	AddSource bool
}

// New creates a configured slog.Logger.
// DevMode produces human-readable text; production produces JSON.
func New(cfg Config) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource || cfg.DevMode,
	}

	var handler slog.Handler
	if cfg.DevMode {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// NewWithRing creates a logger that writes to both the normal output
// and an in-memory ring buffer for diagnostics.
func NewWithRing(cfg Config, ring *RingBuffer) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource || cfg.DevMode,
	}

	var primary slog.Handler
	if cfg.DevMode {
		primary = slog.NewTextHandler(os.Stdout, opts)
	} else {
		primary = slog.NewJSONHandler(os.Stdout, opts)
	}

	handler := &ringHandler{
		primary: primary,
		ring:    ring,
		level:   cfg.Level,
	}

	return slog.New(handler)
}
