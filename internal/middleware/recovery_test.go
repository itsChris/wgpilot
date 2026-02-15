package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRecovery_PanicReturns500WithRequestID(t *testing.T) {
	logger := discardLogger()

	// Chain: Recovery → RequestID → handler (matches production order).
	handler := Recovery(logger)(
		RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code INTERNAL_ERROR, got %q", resp.Error.Code)
	}
	// RequestID middleware sets X-Request-ID before the panic,
	// Recovery reads it from the response header.
	if resp.Error.RequestID == "" {
		t.Error("expected request_id in error response")
	}
}

func TestRecovery_NoPanic_PassesThrough(t *testing.T) {
	logger := discardLogger()

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRecovery_PanicDoesNotCrashServer(t *testing.T) {
	logger := discardLogger()

	handler := Recovery(logger)(
		RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		})),
	)

	// Run multiple requests — server should keep serving.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("request %d: expected 500, got %d", i, w.Code)
		}
	}
}

func TestRecovery_RequestIDFromHeader(t *testing.T) {
	logger := discardLogger()

	// Simulate RequestID middleware by setting the header manually.
	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "req_known12345")
		panic("with id")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp struct {
		Error struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.RequestID != "req_known12345" {
		t.Errorf("expected req_known12345, got %q", resp.Error.RequestID)
	}
}
