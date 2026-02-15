package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// RegisterFrontend configures the server to serve an embedded SPA filesystem.
// Static files are served directly; all other paths fall back to index.html
// to support client-side routing.
func (s *Server) RegisterFrontend(frontendFS fs.FS) {
	fileServer := http.FileServer(http.FS(frontendFS))

	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Skip API and known backend routes.
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") ||
			path == "/health" ||
			path == "/metrics" {
			http.NotFound(w, r)
			return
		}

		// Try to serve the exact file. If it exists, serve it.
		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}
		if _, err := fs.Stat(frontendFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fall back to index.html for SPA client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
