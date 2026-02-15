package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogger_LogsRequest(t *testing.T) {
	logger := discardLogger()

	handler := RequestLogger(logger, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequestLogger_CapturesStatusCode(t *testing.T) {
	logger := discardLogger()

	handler := RequestLogger(logger, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/missing", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRequestLogger_DevMode_PreservesBody(t *testing.T) {
	logger := discardLogger()

	var receivedBody string
	handler := RequestLogger(logger, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 128)
		n, _ := r.Body.Read(b)
		receivedBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"test":"data"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if receivedBody != body {
		t.Errorf("expected body %q, got %q", body, receivedBody)
	}
}
