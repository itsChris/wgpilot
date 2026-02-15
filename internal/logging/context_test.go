package logging

import (
	"context"
	"strings"
	"testing"
)

func TestRequestID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if id := RequestID(ctx); id != "" {
		t.Fatalf("expected empty request ID, got %q", id)
	}

	ctx = WithRequestID(ctx, "req_abc123def456")
	if id := RequestID(ctx); id != "req_abc123def456" {
		t.Fatalf("expected %q, got %q", "req_abc123def456", id)
	}
}

func TestTaskID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if id := TaskID(ctx); id != "" {
		t.Fatalf("expected empty task ID, got %q", id)
	}

	ctx = WithTaskID(ctx, "task_poll_1234567890")
	if id := TaskID(ctx); id != "task_poll_1234567890" {
		t.Fatalf("expected %q, got %q", "task_poll_1234567890", id)
	}
}

func TestGenerateRequestID_Format(t *testing.T) {
	id := GenerateRequestID()
	if !strings.HasPrefix(id, "req_") {
		t.Fatalf("expected prefix req_, got %q", id)
	}
	// "req_" + 12 hex chars = 16 total
	if len(id) != 16 {
		t.Fatalf("expected length 16, got %d for %q", len(id), id)
	}
}

func TestGenerateTaskID_Format(t *testing.T) {
	id := GenerateTaskID("poll")
	if !strings.HasPrefix(id, "task_poll_") {
		t.Fatalf("expected prefix task_poll_, got %q", id)
	}
}

func TestLogAttrsFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	attrs := LogAttrsFromContext(ctx)
	if len(attrs) != 0 {
		t.Fatalf("expected 0 attrs, got %d", len(attrs))
	}
}

func TestLogAttrsFromContext_Both(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req_test")
	ctx = WithTaskID(ctx, "task_test")

	attrs := LogAttrsFromContext(ctx)
	if len(attrs) != 2 {
		t.Fatalf("expected 2 attrs, got %d", len(attrs))
	}
}

func TestGenerateRequestID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateRequestID()
		if seen[id] {
			t.Fatalf("duplicate request ID: %s", id)
		}
		seen[id] = true
	}
}
