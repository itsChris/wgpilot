package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
)

func createTestUser(t *testing.T, d *db.DB, username, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	_, err = d.CreateUser(context.Background(), &db.User{
		Username:     username,
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
}

func TestHandleLogin_Success(t *testing.T) {
	srv := newTestServer(t)
	createTestUser(t, srv.db, "admin", "correctpassword")

	body := `{"username":"admin","password":"correctpassword"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check JWT in cookie.
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == auth.CookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Error("cookie should be SameSite=Strict")
	}

	// Validate the JWT.
	claims, err := srv.jwtService.Validate(sessionCookie.Value)
	if err != nil {
		t.Fatalf("token should be valid: %v", err)
	}
	if claims.Username != "admin" {
		t.Errorf("expected username=admin, got %q", claims.Username)
	}

	// Check response body.
	var resp loginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.User.Username != "admin" {
		t.Errorf("expected username=admin in response, got %q", resp.User.Username)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	srv := newTestServer(t)
	createTestUser(t, srv.db, "admin", "correctpassword")

	body := `{"username":"admin","password":"wrongpassword"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	// No session cookie.
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName && c.MaxAge > 0 {
			t.Error("should not set session cookie on failed login")
		}
	}
}

func TestHandleLogin_UserNotFound(t *testing.T) {
	srv := newTestServer(t)

	body := `{"username":"nonexistent","password":"anything"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleLogin_RateLimit(t *testing.T) {
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

	// Send 5 failed attempts.
	for i := 0; i < 5; i++ {
		body := `{"username":"admin","password":"wrong"}`
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = "192.168.1.100:12345"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}

	// 6th attempt should be rate limited.
	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestHandleSetup_Success(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// Clear setup_complete so the old setup endpoint works.
	if err := srv.db.DeleteSetting(ctx, "setup_complete"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}

	// Store OTP hash.
	otp := "testsetuppassword"
	otpHash, err := auth.HashPassword(otp)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := srv.db.SetSetting(ctx, "setup_otp", otpHash); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	body := `{"otp":"testsetuppassword","username":"myadmin","password":"mynewpassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Check that a session cookie was set.
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected session cookie after setup")
	}

	// Verify admin user was created.
	user, err := srv.db.GetUserByUsername(ctx, "myadmin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user == nil {
		t.Fatal("expected admin user to be created")
	}

	// Verify OTP was deleted.
	otpVal, err := srv.db.GetSetting(ctx, "setup_otp")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if otpVal != "" {
		t.Error("OTP should be deleted after setup")
	}

	// Verify setup_complete is set.
	complete, err := srv.db.GetSetting(ctx, "setup_complete")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if complete != "true" {
		t.Errorf("expected setup_complete=true, got %q", complete)
	}
}

func TestHandleSetup_AlreadyComplete(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	if err := srv.db.SetSetting(ctx, "setup_complete", "true"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	body := `{"otp":"anything","username":"admin","password":"somepassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetup_InvalidOTP(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// Clear setup_complete so the old setup endpoint works.
	if err := srv.db.DeleteSetting(ctx, "setup_complete"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}

	otpHash, err := auth.HashPassword("correct-otp")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := srv.db.SetSetting(ctx, "setup_otp", otpHash); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	body := `{"otp":"wrong-otp","username":"admin","password":"somepassword123"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetup_PasswordTooShort(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// Clear setup_complete so the old setup endpoint works.
	if err := srv.db.DeleteSetting(ctx, "setup_complete"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}

	otpHash, err := auth.HashPassword("correct-otp")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := srv.db.SetSetting(ctx, "setup_otp", otpHash); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	body := `{"otp":"correct-otp","username":"admin","password":"short"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLogout(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Check cookie is cleared.
	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected session cookie to be cleared")
	}
}

func TestExpiredJWT_ProtectedEndpoint(t *testing.T) {
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

	// Use nanosecond TTL so token expires immediately.
	jwtSvc, err := auth.NewJWTService(newTestJWTSecret(), time.Nanosecond, logger)
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

	token, err := jwtSvc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// Hit a protected endpoint.
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecurityHeaders_OnAuthEndpoints(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("expected X-Content-Type-Options=nosniff, got %q", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("expected X-Frame-Options=DENY, got %q", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("expected Content-Security-Policy header")
	}
}
