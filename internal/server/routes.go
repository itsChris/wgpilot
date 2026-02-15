package server

import (
	"fmt"
	"net/http"
	"time"

	apperr "github.com/itsChris/wgpilot/internal/errors"
	servermw "github.com/itsChris/wgpilot/internal/server/middleware"
)

// registerRoutes wires all API endpoints. Handlers that are not yet
// implemented return 501 Not Implemented.
func (s *Server) registerRoutes() {
	protected := servermw.RequireAuth(s.jwtService, s.sessions, s.logger)

	// ── Public routes (no auth) ───────────────────────────────────────

	// Health check.
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Prometheus metrics (unauthenticated).
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Auth.
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// ── Protected routes ──────────────────────────────────────────────

	// Auth (protected).
	s.mux.Handle("GET /api/auth/me", protected(http.HandlerFunc(s.handleMe)))
	s.mux.Handle("PUT /api/auth/password", protected(http.HandlerFunc(s.notImplemented)))

	// Setup (first-run only, but still requires no auth per spec — gated internally).
	s.mux.HandleFunc("GET /api/setup/status", s.notImplemented)
	s.mux.HandleFunc("POST /api/setup/admin", s.notImplemented)
	s.mux.HandleFunc("PUT /api/setup/server", s.notImplemented)
	s.mux.HandleFunc("POST /api/setup/network", s.notImplemented)
	s.mux.HandleFunc("POST /api/setup/peer", s.notImplemented)
	s.mux.HandleFunc("POST /api/setup/import", s.notImplemented)

	// Networks.
	s.mux.Handle("GET /api/networks", protected(http.HandlerFunc(s.handleListNetworks)))
	s.mux.Handle("POST /api/networks", protected(http.HandlerFunc(s.handleCreateNetwork)))
	s.mux.Handle("GET /api/networks/{id}", protected(http.HandlerFunc(s.handleGetNetwork)))
	s.mux.Handle("PUT /api/networks/{id}", protected(http.HandlerFunc(s.handleUpdateNetwork)))
	s.mux.Handle("DELETE /api/networks/{id}", protected(http.HandlerFunc(s.handleDeleteNetwork)))
	s.mux.Handle("POST /api/networks/{id}/enable", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/networks/{id}/disable", protected(http.HandlerFunc(s.notImplemented)))

	// Peers.
	s.mux.Handle("GET /api/networks/{id}/peers", protected(http.HandlerFunc(s.handleListPeers)))
	s.mux.Handle("POST /api/networks/{id}/peers", protected(http.HandlerFunc(s.handleCreatePeer)))
	s.mux.Handle("GET /api/networks/{id}/peers/{pid}", protected(http.HandlerFunc(s.handleGetPeer)))
	s.mux.Handle("PUT /api/networks/{id}/peers/{pid}", protected(http.HandlerFunc(s.handleUpdatePeer)))
	s.mux.Handle("DELETE /api/networks/{id}/peers/{pid}", protected(http.HandlerFunc(s.handleDeletePeer)))
	s.mux.Handle("POST /api/networks/{id}/peers/{pid}/enable", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/networks/{id}/peers/{pid}/disable", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("GET /api/networks/{id}/peers/{pid}/config", protected(http.HandlerFunc(s.handlePeerConfig)))
	s.mux.Handle("GET /api/networks/{id}/peers/{pid}/qr", protected(http.HandlerFunc(s.handlePeerQR)))

	// Network bridges.
	s.mux.Handle("GET /api/bridges", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/bridges", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("GET /api/bridges/{id}", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("PUT /api/bridges/{id}", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("DELETE /api/bridges/{id}", protected(http.HandlerFunc(s.notImplemented)))

	// Status & Monitoring.
	s.mux.Handle("GET /api/status", protected(http.HandlerFunc(s.handleStatus)))
	s.mux.Handle("GET /api/networks/{id}/events", protected(http.HandlerFunc(s.handleSSEEvents)))
	s.mux.Handle("GET /api/networks/{id}/stats", protected(http.HandlerFunc(s.notImplemented)))

	// Debug (admin-only).
	s.mux.Handle("GET /api/debug/info", protected(http.HandlerFunc(s.handleDebugInfo)))
	s.mux.Handle("GET /api/debug/logs", protected(http.HandlerFunc(s.handleDebugLogs)))

	// Settings.
	s.mux.Handle("GET /api/settings", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("PUT /api/settings", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("GET /api/settings/tls", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/settings/tls/test", protected(http.HandlerFunc(s.notImplemented)))

	// Alerts.
	s.mux.Handle("GET /api/alerts", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/alerts", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("PUT /api/alerts/{id}", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("DELETE /api/alerts/{id}", protected(http.HandlerFunc(s.notImplemented)))

	// System.
	s.mux.Handle("GET /api/system/info", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/system/backup", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/system/restore", protected(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("GET /api/audit-log", protected(http.HandlerFunc(s.notImplemented)))
}

// handleHealth is the unauthenticated health check endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)

	networks, err := s.db.ListNetworks(r.Context())
	networkInfo := map[string]int{"total": 0, "healthy": 0, "degraded": 0}
	dbStatus := "ok"
	if err != nil {
		dbStatus = "error"
	} else {
		networkInfo["total"] = len(networks)
		healthy := 0
		for _, n := range networks {
			if n.Enabled {
				healthy++
			}
		}
		networkInfo["healthy"] = healthy
		networkInfo["degraded"] = networkInfo["total"] - healthy
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "healthy",
		"version":  s.version,
		"uptime":   formatDuration(uptime),
		"networks": networkInfo,
		"database": dbStatus,
	})
}

// notImplemented returns 501 for endpoints that are registered but not
// yet implemented.
func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeError(w, r,
		fmt.Errorf("endpoint %s %s is not yet implemented", r.Method, r.URL.Path),
		apperr.ErrInternal,
		http.StatusNotImplemented,
		s.devMode,
	)
}

// formatDuration formats a duration as a human-readable string like "14d 3h 22m".
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
