package auth

import (
	"fmt"
	"log/slog"
	"net/http"
)

// CookieName is the name of the session cookie.
const CookieName = "session"

// SessionManager handles HTTP cookie-based session management.
type SessionManager struct {
	secure bool
	logger *slog.Logger
}

// NewSessionManager creates a session manager.
// Set secure=true to require HTTPS (Secure flag on cookie).
func NewSessionManager(secure bool, logger *slog.Logger) (*SessionManager, error) {
	return &SessionManager{
		secure: secure,
		logger: logger,
	}, nil
}

// SetCookie writes the session JWT as an HttpOnly cookie.
func (m *SessionManager) SetCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearCookie removes the session cookie by setting MaxAge to -1.
func (m *SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// GetToken extracts the session token from the request cookie.
func (m *SessionManager) GetToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", fmt.Errorf("session: get cookie: %w", err)
	}
	return cookie.Value, nil
}
