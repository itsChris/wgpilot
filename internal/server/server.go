package server

import (
	"log/slog"
	"net/http"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/middleware"
	servermw "github.com/itsChris/wgpilot/internal/server/middleware"
)

// Server is the HTTP server that wires together all subsystems.
type Server struct {
	db          *db.DB
	logger      *slog.Logger
	jwtService  *auth.JWTService
	sessions    *auth.SessionManager
	rateLimiter *auth.LoginRateLimiter
	devMode     bool
	handler     http.Handler
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

// New creates a Server, registers all routes, and builds the middleware chain.
//
// Middleware order (outermost → innermost):
//
//	recovery → security_headers → request_id → request_logger → max_body → auth → handler
//
// Auth is applied per-route rather than globally so public endpoints
// (health, login, setup) bypass it.
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

	// Build middleware chain (applied inside-out, listed outside-in).
	var handler http.Handler = s.mux
	handler = middleware.MaxBody(middleware.DefaultMaxBodySize)(handler)
	handler = middleware.RequestLogger(cfg.Logger, cfg.DevMode)(handler)
	handler = middleware.RequestID(handler)
	handler = servermw.SecurityHeaders(cfg.DevMode)(handler)
	handler = middleware.Recovery(cfg.Logger)(handler)

	s.handler = handler
	return s, nil
}

// ServeHTTP implements http.Handler by delegating to the middleware chain.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}
