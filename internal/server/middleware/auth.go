package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
)

// APIKeyStore abstracts API key lookups for the auth middleware.
type APIKeyStore interface {
	GetAPIKeyByHash(ctx context.Context, hash string) (id int64, userID int64, role string, expiresAt *time.Time, err error)
	UpdateAPIKeyLastUsed(ctx context.Context, id int64) error
}

// RequireAuth returns middleware that validates the JWT from the session cookie
// or an API key from the Authorization header, and injects the user claims
// into the request context.
func RequireAuth(jwtSvc *auth.JWTService, sessions *auth.SessionManager, logger *slog.Logger, apiKeyStore ...APIKeyStore) func(http.Handler) http.Handler {
	var keyStore APIKeyStore
	if len(apiKeyStore) > 0 {
		keyStore = apiKeyStore[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try API key from Authorization header first.
			if keyStore != nil {
				if bearer := r.Header.Get("Authorization"); strings.HasPrefix(bearer, "Bearer wgp_") {
					apiKey := strings.TrimPrefix(bearer, "Bearer ")
					hash := auth.HashAPIKey(apiKey)

					id, userID, role, expiresAt, err := keyStore.GetAPIKeyByHash(r.Context(), hash)
					if err == nil && id > 0 {
						// Check expiry.
						if expiresAt != nil && expiresAt.Before(time.Now()) {
							writeAuthError(w, "api key expired", "SESSION_EXPIRED", http.StatusUnauthorized)
							return
						}

						// Update last used (non-blocking).
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
							defer cancel()
							keyStore.UpdateAPIKeyLastUsed(ctx, id)
						}()

						claims := &auth.Claims{
							Username: "api-key",
							Role:     role,
						}
						claims.Subject = strconv.FormatInt(userID, 10)

						ctx := auth.WithUser(r.Context(), claims)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			// Fall back to JWT cookie.
			token, err := sessions.GetToken(r)
			if err != nil {
				logger.Warn("auth_no_session",
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path,
					"component", "auth",
				)
				writeAuthError(w, "unauthorized", "UNAUTHORIZED", http.StatusUnauthorized)
				return
			}

			claims, err := jwtSvc.Validate(token)
			if err != nil {
				logger.Warn("auth_invalid_token",
					"remote_addr", r.RemoteAddr,
					"path", r.URL.Path,
					"error", err,
					"component", "auth",
				)
				writeAuthError(w, "session expired or invalid", "SESSION_EXPIRED", http.StatusUnauthorized)
				return
			}

			ctx := auth.WithUser(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that checks the authenticated user has one of
// the allowed roles. Must be chained after RequireAuth.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.UserFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, "unauthorized", "UNAUTHORIZED", http.StatusUnauthorized)
				return
			}
			if !allowed[claims.Role] {
				writeAuthError(w, "insufficient permissions", "FORBIDDEN", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthError(w http.ResponseWriter, msg, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
		"code":  code,
	})
}
