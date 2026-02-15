package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsChris/wgpilot/internal/logging"
)

func TestRequestID_GeneratedAndInHeader(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	id := w.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
	if !strings.HasPrefix(id, "req_") {
		t.Errorf("expected request ID to start with req_, got %q", id)
	}
	if len(id) != 16 { // "req_" + 12 hex chars
		t.Errorf("expected request ID length 16, got %d (%q)", len(id), id)
	}
}

func TestRequestID_PropagatesToContext(t *testing.T) {
	var contextID string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID = logging.RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	headerID := w.Header().Get("X-Request-ID")
	if contextID == "" {
		t.Fatal("expected request ID in context")
	}
	if contextID != headerID {
		t.Errorf("context ID %q != header ID %q", contextID, headerID)
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	var ids []string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids = append(ids, w.Header().Get("X-Request-ID"))
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate request ID: %q", id)
		}
		seen[id] = true
	}
}
