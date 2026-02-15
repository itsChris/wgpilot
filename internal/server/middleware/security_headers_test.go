package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_AllPresent(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"X-XSS-Protection", "0"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
		{"Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'"},
	}

	for _, tt := range tests {
		got := w.Header().Get(tt.header)
		if got != tt.expected {
			t.Errorf("%s: expected %q, got %q", tt.header, tt.expected, got)
		}
	}
}

func TestSecurityHeaders_NoHSTS_WithoutTLS(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should not be set without TLS, got %q", got)
	}
}

func TestSecurityHeaders_NoCORS_Production(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS should not be set in production, got %q", got)
	}
}

func TestSecurityHeaders_CORS_DevMode(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("expected CORS origin http://localhost:5173, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials=true, got %q", got)
	}
}

func TestSecurityHeaders_Preflight_DevMode(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest("OPTIONS", "/api/networks", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
}
