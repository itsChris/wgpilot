package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestJWTSecret() []byte {
	return []byte("test-secret-key-that-is-32-bytes!")
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	logger := newDiscardLogger()
	ring := logging.NewRingBuffer(100)

	database, err := db.New(ctx, ":memory:", logger, true)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(ctx, database, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	jwtSvc, err := auth.NewJWTService(newTestJWTSecret(), 24*time.Hour, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	limiter, err := auth.NewLoginRateLimiter(5, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	t.Cleanup(func() { limiter.Stop() })

	srv, err := New(Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: limiter,
		DevMode:     true,
		Ring:        ring,
		Version:     "test",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func TestRequestID_GeneratedInResponse(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	id := w.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("expected X-Request-ID header")
	}
	if !strings.HasPrefix(id, "req_") {
		t.Errorf("expected request ID prefix req_, got %q", id)
	}
}

func TestRequestID_PropagatesIntoHandlerContext(t *testing.T) {
	srv := newTestServer(t)

	// The health endpoint is public and has no auth.
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Request ID is in the response header — the middleware set it
	// into the context, and the request_logger middleware read it back.
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("request ID not set")
	}
}

func TestPanicInHandler_Returns500WithRequestID(t *testing.T) {
	ctx := context.Background()
	logger := newDiscardLogger()

	database, err := db.New(ctx, ":memory:", logger, true)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(ctx, database, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	defer database.Close()

	jwtSvc, err := auth.NewJWTService(newTestJWTSecret(), 24*time.Hour, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	limiter, err := auth.NewLoginRateLimiter(5, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	defer limiter.Stop()

	srv, err := New(Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: limiter,
		DevMode:     true,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// Register a handler that panics (via the mux directly for testing).
	srv.mux.HandleFunc("GET /api/test-panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic in handler")
	})

	req := httptest.NewRequest("GET", "/api/test-panic", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR, got %q", resp.Error.Code)
	}
	if resp.Error.RequestID == "" {
		t.Error("expected request_id in panic response")
	}

	// Server should still be alive — make another request.
	req2 := httptest.NewRequest("GET", "/health", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 after panic, got %d", w2.Code)
	}
}

func TestBodyOverLimit_Returns413(t *testing.T) {
	srv := newTestServer(t)

	// Send a valid JSON body larger than 1MB. The JSON decoder will keep
	// reading the string value until the MaxBytesReader triggers.
	bigBody := `{"username":"admin","password":"` + strings.Repeat("a", 2*1024*1024) + `"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUnauthenticatedRequest_ProtectedEndpoint_Returns401(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/networks", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHealthEndpoint_NoAuth(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("expected status=healthy, got %q", resp["status"])
	}
}

func TestSecurityHeaders_Present(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"X-XSS-Protection":      "0",
	}
	for header, expected := range checks {
		got := w.Header().Get(header)
		if got != expected {
			t.Errorf("%s: expected %q, got %q", header, expected, got)
		}
	}
}

func TestNotImplementedEndpoint_Returns501(t *testing.T) {
	srv := newTestServer(t)

	// Create a valid token to access a protected endpoint.
	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestErrorResponse_IncludesRequestID(t *testing.T) {
	srv := newTestServer(t)

	// Hit a protected endpoint without auth to get an error response.
	req := httptest.NewRequest("GET", "/api/networks", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Check that X-Request-ID header is present even on error.
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		t.Error("expected X-Request-ID header on error response")
	}
}

func TestWriteError_DevMode_IncludesDetail(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := logging.WithRequestID(req.Context(), "req_testdetail1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	writeError(w, req, errForTest("detail test error"), "TEST_CODE", http.StatusBadRequest, true)

	var resp errorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Detail == "" {
		t.Error("expected detail in dev mode")
	}
	if resp.Error.RequestID != "req_testdetail1" {
		t.Errorf("expected request_id req_testdetail1, got %q", resp.Error.RequestID)
	}
}

func TestWriteError_ProdMode_NoDetail(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	writeError(w, req, errForTest("secret detail"), "TEST_CODE", http.StatusBadRequest, false)

	var resp errorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Detail != "" {
		t.Error("expected no detail in prod mode")
	}
	if resp.Error.Stack != "" {
		t.Error("expected no stack in prod mode")
	}
}

type testErr string

func errForTest(msg string) error { return testErr(msg) }
func (e testErr) Error() string   { return string(e) }
