package middleware

import (
	"net/http"

	"github.com/itsChris/wgpilot/internal/logging"
)

// RequestID generates a unique request ID, injects it into the request
// context, and sets the X-Request-ID response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := logging.GenerateRequestID()
		ctx := logging.WithRequestID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
