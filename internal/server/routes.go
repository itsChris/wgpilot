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
	apiKeys := &apiKeyStoreAdapter{db: s.db}
	protected := servermw.RequireAuth(s.jwtService, s.sessions, s.logger, apiKeys)

	// guarded wraps a handler with both auth and setup guard: the request
	// must be authenticated AND setup must be complete.
	guarded := func(h http.Handler) http.Handler {
		return protected(s.setupGuard(h))
	}

	// adminOnly wraps a handler with auth + setup guard + admin role check.
	adminGuard := servermw.RequireRole("admin")
	adminOnly := func(h http.Handler) http.Handler {
		return protected(s.setupGuard(adminGuard(h)))
	}

	// ── Public routes (no auth) ───────────────────────────────────────

	// Health check.
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Prometheus metrics (unauthenticated).
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Auth.
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// ── Setup routes (no auth — gated internally by OTP/step checks) ─
	s.mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/setup/step/1", s.handleSetupStep1)
	s.mux.Handle("POST /api/setup/step/2", protected(http.HandlerFunc(s.handleSetupStep2)))
	s.mux.Handle("POST /api/setup/step/3", protected(http.HandlerFunc(s.handleSetupStep3)))
	s.mux.Handle("POST /api/setup/step/4", protected(http.HandlerFunc(s.handleSetupStep4)))
	s.mux.HandleFunc("GET /api/setup/detect-ip", s.handleDetectPublicIP)
	s.mux.Handle("POST /api/setup/import", protected(http.HandlerFunc(s.handleImportConfig)))

	// ── Protected + guarded routes (require auth + setup complete) ────

	// Auth (protected only — no setup guard needed).
	s.mux.Handle("GET /api/auth/me", protected(http.HandlerFunc(s.handleMe)))
	s.mux.Handle("PUT /api/auth/password", protected(http.HandlerFunc(s.handleChangePassword)))

	// Networks.
	s.mux.Handle("GET /api/networks", guarded(http.HandlerFunc(s.handleListNetworks)))
	s.mux.Handle("POST /api/networks", guarded(http.HandlerFunc(s.handleCreateNetwork)))
	s.mux.Handle("GET /api/networks/{id}", guarded(http.HandlerFunc(s.handleGetNetwork)))
	s.mux.Handle("PUT /api/networks/{id}", guarded(http.HandlerFunc(s.handleUpdateNetwork)))
	s.mux.Handle("DELETE /api/networks/{id}", guarded(http.HandlerFunc(s.handleDeleteNetwork)))
	s.mux.Handle("POST /api/networks/{id}/enable", guarded(http.HandlerFunc(s.handleEnableNetwork)))
	s.mux.Handle("POST /api/networks/{id}/disable", guarded(http.HandlerFunc(s.handleDisableNetwork)))
	s.mux.Handle("GET /api/networks/{id}/export", guarded(http.HandlerFunc(s.handleExportNetwork)))

	// Peers.
	s.mux.Handle("GET /api/networks/{id}/peers", guarded(http.HandlerFunc(s.handleListPeers)))
	s.mux.Handle("POST /api/networks/{id}/peers", guarded(http.HandlerFunc(s.handleCreatePeer)))
	s.mux.Handle("GET /api/networks/{id}/peers/{pid}", guarded(http.HandlerFunc(s.handleGetPeer)))
	s.mux.Handle("PUT /api/networks/{id}/peers/{pid}", guarded(http.HandlerFunc(s.handleUpdatePeer)))
	s.mux.Handle("DELETE /api/networks/{id}/peers/{pid}", guarded(http.HandlerFunc(s.handleDeletePeer)))
	s.mux.Handle("POST /api/networks/{id}/peers/{pid}/enable", guarded(http.HandlerFunc(s.handleEnablePeer)))
	s.mux.Handle("POST /api/networks/{id}/peers/{pid}/disable", guarded(http.HandlerFunc(s.handleDisablePeer)))
	s.mux.Handle("GET /api/networks/{id}/peers/{pid}/config", guarded(http.HandlerFunc(s.handlePeerConfig)))
	s.mux.Handle("GET /api/networks/{id}/peers/{pid}/qr", guarded(http.HandlerFunc(s.handlePeerQR)))

	// Network bridges.
	s.mux.Handle("GET /api/bridges", guarded(http.HandlerFunc(s.handleListBridges)))
	s.mux.Handle("POST /api/bridges", guarded(http.HandlerFunc(s.handleCreateBridge)))
	s.mux.Handle("GET /api/bridges/{id}", guarded(http.HandlerFunc(s.handleGetBridge)))
	s.mux.Handle("PUT /api/bridges/{id}", guarded(http.HandlerFunc(s.handleUpdateBridge)))
	s.mux.Handle("DELETE /api/bridges/{id}", guarded(http.HandlerFunc(s.handleDeleteBridge)))

	// Status & Monitoring.
	s.mux.Handle("GET /api/status", guarded(http.HandlerFunc(s.handleStatus)))
	s.mux.Handle("GET /api/networks/{id}/events", guarded(http.HandlerFunc(s.handleSSEEvents)))
	s.mux.Handle("GET /api/networks/{id}/stats", guarded(http.HandlerFunc(s.handleNetworkStats)))

	// Debug (admin-only).
	s.mux.Handle("GET /api/debug/info", adminOnly(http.HandlerFunc(s.handleDebugInfo)))
	s.mux.Handle("GET /api/debug/logs", adminOnly(http.HandlerFunc(s.handleDebugLogs)))

	// User management (admin-only).
	s.mux.Handle("GET /api/users", adminOnly(http.HandlerFunc(s.handleListUsers)))
	s.mux.Handle("POST /api/users", adminOnly(http.HandlerFunc(s.handleCreateUser)))
	s.mux.Handle("DELETE /api/users/{id}", adminOnly(http.HandlerFunc(s.handleDeleteUser)))

	// Settings.
	s.mux.Handle("GET /api/settings", guarded(http.HandlerFunc(s.handleGetSettings)))
	s.mux.Handle("PUT /api/settings", guarded(http.HandlerFunc(s.handleUpdateSettings)))
	s.mux.Handle("GET /api/settings/tls", guarded(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/settings/tls/test", guarded(http.HandlerFunc(s.notImplemented)))

	// Alerts.
	s.mux.Handle("GET /api/alerts", guarded(http.HandlerFunc(s.handleListAlerts)))
	s.mux.Handle("POST /api/alerts", guarded(http.HandlerFunc(s.handleCreateAlert)))
	s.mux.Handle("PUT /api/alerts/{id}", guarded(http.HandlerFunc(s.handleUpdateAlert)))
	s.mux.Handle("DELETE /api/alerts/{id}", guarded(http.HandlerFunc(s.handleDeleteAlert)))

	// API Keys.
	s.mux.Handle("GET /api/api-keys", guarded(http.HandlerFunc(s.handleListAPIKeys)))
	s.mux.Handle("POST /api/api-keys", guarded(http.HandlerFunc(s.handleCreateAPIKey)))
	s.mux.Handle("DELETE /api/api-keys/{id}", guarded(http.HandlerFunc(s.handleDeleteAPIKey)))

	// System.
	s.mux.Handle("GET /api/system/info", guarded(http.HandlerFunc(s.handleSystemInfo)))
	s.mux.Handle("POST /api/system/backup", guarded(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("POST /api/system/restore", guarded(http.HandlerFunc(s.notImplemented)))
	s.mux.Handle("GET /api/audit-log", guarded(http.HandlerFunc(s.handleAuditLog)))
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
