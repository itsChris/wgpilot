package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
	"github.com/itsChris/wgpilot/internal/server/middleware"
)

// Server is the HTTP server that wires together all subsystems.
type Server struct {
	db          *db.DB
	logger      *slog.Logger
	jwtService  *auth.JWTService
	sessions    *auth.SessionManager
	rateLimiter *auth.LoginRateLimiter
	devMode     bool
	mux         *http.ServeMux
}

// Config holds the dependencies for creating a new Server.
type Config struct {
	DB          *db.DB
	Logger      *slog.Logger
	JWTService  *auth.JWTService
	Sessions    *auth.SessionManager
	RateLimiter *auth.LoginRateLimiter
	DevMode     bool
}

// New creates a Server and registers all routes.
func New(cfg Config) (*Server, error) {
	s := &Server{
		db:          cfg.DB,
		logger:      cfg.Logger,
		jwtService:  cfg.JWTService,
		sessions:    cfg.Sessions,
		rateLimiter: cfg.RateLimiter,
		devMode:     cfg.DevMode,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := middleware.SecurityHeaders(s.devMode)(s.mux)
	handler.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	// Public auth routes.
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// Health check (public).
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Protected routes are registered with the auth middleware.
	protected := middleware.RequireAuth(s.jwtService, s.sessions, s.logger)

	// Example protected endpoint placeholder.
	s.mux.Handle("GET /api/auth/me", protected(http.HandlerFunc(s.handleMe)))
}

// handleMe returns the current authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := auth.UserFromContext(r.Context())
	if claims == nil {
		s.writeError(w, r, "unauthorized", "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":       claims.Subject,
			"username": claims.Username,
			"role":     claims.Role,
		},
	})
}

type errorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, msg string, code string, status int) {
	resp := errorResponse{
		Error:     msg,
		Code:      code,
		RequestID: logging.RequestID(r.Context()),
	}
	writeJSON(w, status, resp)
}
