package middleware

import (
	"net/http"
)

// DefaultMaxBodySize is 1 MB.
const DefaultMaxBodySize = 1 << 20 // 1 MB

// MaxBody limits request body size to the specified number of bytes.
// Returns 413 Request Entity Too Large if the body exceeds the limit.
func MaxBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
