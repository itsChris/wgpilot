package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testSecret() []byte {
	return []byte("test-secret-key-that-is-32-bytes!")
}

func TestRequireAuth_ValidToken(t *testing.T) {
	logger := testLogger()
	jwtSvc, err := auth.NewJWTService(testSecret(), 24*time.Hour, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	token, err := jwtSvc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var gotClaims *auth.Claims
	handler := RequireAuth(jwtSvc, sessions, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/networks", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotClaims == nil {
		t.Fatal("expected claims in context")
	}
	if gotClaims.Username != "admin" {
		t.Errorf("expected username=admin, got %q", gotClaims.Username)
	}
}

func TestRequireAuth_NoToken(t *testing.T) {
	logger := testLogger()
	jwtSvc, err := auth.NewJWTService(testSecret(), 24*time.Hour, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	handler := RequireAuth(jwtSvc, sessions, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without auth")
	}))

	req := httptest.NewRequest("GET", "/api/networks", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	logger := testLogger()
	jwtSvc, err := auth.NewJWTService(testSecret(), time.Nanosecond, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	token, err := jwtSvc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	handler := RequireAuth(jwtSvc, sessions, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with expired token")
	}))

	req := httptest.NewRequest("GET", "/api/networks", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	logger := testLogger()
	jwtSvc, err := auth.NewJWTService(testSecret(), 24*time.Hour, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	handler := RequireAuth(jwtSvc, sessions, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid token")
	}))

	req := httptest.NewRequest("GET", "/api/networks", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "garbage.token.value"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
