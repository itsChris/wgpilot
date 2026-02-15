package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetCookie(t *testing.T) {
	sm, err := NewSessionManager(false, testLogger())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	w := httptest.NewRecorder()
	sm.SetCookie(w, "test-token", 86400)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	c := cookies[0]
	if c.Name != CookieName {
		t.Errorf("expected cookie name %q, got %q", CookieName, c.Name)
	}
	if c.Value != "test-token" {
		t.Errorf("expected value %q, got %q", "test-token", c.Value)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly=true")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("expected Path=/, got %q", c.Path)
	}
	if c.MaxAge != 86400 {
		t.Errorf("expected MaxAge=86400, got %d", c.MaxAge)
	}
}

func TestSetCookie_Secure(t *testing.T) {
	sm, err := NewSessionManager(true, testLogger())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	w := httptest.NewRecorder()
	sm.SetCookie(w, "token", 3600)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if !cookies[0].Secure {
		t.Error("expected Secure=true")
	}
}

func TestClearCookie(t *testing.T) {
	sm, err := NewSessionManager(false, testLogger())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	w := httptest.NewRecorder()
	sm.ClearCookie(w)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	c := cookies[0]
	if c.Name != CookieName {
		t.Errorf("expected cookie name %q, got %q", CookieName, c.Name)
	}
	if c.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 for deletion, got %d", c.MaxAge)
	}
}

func TestGetToken_Present(t *testing.T) {
	sm, err := NewSessionManager(false, testLogger())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "my-jwt-token"})

	token, err := sm.GetToken(req)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token != "my-jwt-token" {
		t.Errorf("expected %q, got %q", "my-jwt-token", token)
	}
}

func TestGetToken_Missing(t *testing.T) {
	sm, err := NewSessionManager(false, testLogger())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)

	_, err = sm.GetToken(req)
	if err == nil {
		t.Error("GetToken should fail when no cookie is present")
	}
}
