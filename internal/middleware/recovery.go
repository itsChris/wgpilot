package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery catches panics in downstream handlers, logs the panic with a
// full stack trace, and returns a 500 JSON error response. The request ID
// is read from the X-Request-ID response header (set by the RequestID
// middleware, which runs inside Recovery in the chain).
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()
					requestID := w.Header().Get("X-Request-ID")

					logger.Error("panic_recovered",
						"panic", fmt.Sprintf("%v", rec),
						"stack", string(stack),
						"method", r.Method,
						"path", r.URL.Path,
						"request_id", requestID,
						"component", "http",
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(w,
						`{"error":{"code":"INTERNAL_ERROR","message":"internal error","request_id":%q}}`,
						requestID,
					)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
