package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/itsChris/wgpilot/internal/auth"
)

// RequireAuth returns middleware that validates the JWT from the session cookie
// and injects the user claims into the request context.
func RequireAuth(jwtSvc *auth.JWTService, sessions *auth.SessionManager, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func writeAuthError(w http.ResponseWriter, msg, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
		"code":  code,
	})
}
