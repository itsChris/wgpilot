package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/logging"
)

// RequestLogger logs every HTTP request and response. In dev mode,
// request and response bodies are included for debugging.
func RequestLogger(logger *slog.Logger, devMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := logging.RequestID(r.Context())

			attrs := []any{
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			}

			if devMode && r.Body != nil && r.ContentLength > 0 && r.ContentLength < 1_000_000 {
				body, err := io.ReadAll(r.Body)
				if err == nil {
					r.Body = io.NopCloser(bytes.NewReader(body))
					attrs = append(attrs, "request_body", string(body))
				}
			}

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			attrs = append(attrs,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"bytes_written", wrapped.bytesWritten,
			)

			if devMode && wrapped.body.Len() > 0 && wrapped.body.Len() < 1_000_000 {
				attrs = append(attrs, "response_body", wrapped.body.String())
			}

			if claims := auth.UserFromContext(r.Context()); claims != nil {
				attrs = append(attrs, "user", claims.Username)
			}

			level := slog.LevelInfo
			if wrapped.statusCode >= 500 {
				level = slog.LevelError
			} else if wrapped.statusCode >= 400 {
				level = slog.LevelWarn
			}

			logger.Log(r.Context(), level, "http_request", attrs...)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code, bytes written,
// and optionally the response body.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	body         bytes.Buffer
	wroteHeader  bool
}

func (w *responseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

// Unwrap lets http.ResponseController access the underlying ResponseWriter.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
