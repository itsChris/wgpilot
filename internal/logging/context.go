package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	taskIDKey    contextKey = "task_id"
)

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID extracts the request ID from the context.
// Returns empty string if not set.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// GenerateRequestID creates a new request ID in the format "req_<12 hex chars>".
func GenerateRequestID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(b)
}

// WithTaskID stores a task ID in the context.
func WithTaskID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, taskIDKey, id)
}

// TaskID extracts the task ID from the context.
func TaskID(ctx context.Context) string {
	id, _ := ctx.Value(taskIDKey).(string)
	return id
}

// GenerateTaskID creates a new task ID in the format "task_<name>_<unix timestamp>".
func GenerateTaskID(name string) string {
	return fmt.Sprintf("task_%s_%d", name, time.Now().Unix())
}

// LogAttrsFromContext extracts request_id and task_id from context
// and returns them as slog attributes. Only non-empty values are included.
func LogAttrsFromContext(ctx context.Context) []slog.Attr {
	var attrs []slog.Attr
	if id := RequestID(ctx); id != "" {
		attrs = append(attrs, slog.String("request_id", id))
	}
	if id := TaskID(ctx); id != "" {
		attrs = append(attrs, slog.String("task_id", id))
	}
	return attrs
}
