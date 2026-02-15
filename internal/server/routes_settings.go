package server

import (
	"fmt"
	"net/http"

	apperr "github.com/itsChris/wgpilot/internal/errors"
)

// sensitiveSettings are filtered from the GET /api/settings response.
var sensitiveSettings = map[string]bool{
	"jwt_secret":       true,
	"setup_otp":        true,
	"setup_step1_done": true,
	"setup_step2_done": true,
	"setup_step3_done": true,
	"setup_network_id": true,
	"setup_complete":   true,
}

// allowedSettings are the only keys that can be updated via PUT /api/settings.
var allowedSettings = map[string]bool{
	"public_ip":    true,
	"hostname":     true,
	"dns_servers":  true,
	"smtp_host":    true,
	"smtp_port":    true,
	"smtp_user":    true,
	"smtp_pass":    true,
	"smtp_from":    true,
	"smtp_tls":     true,
	"alert_email":  true,
}

// handleGetSettings returns all non-sensitive settings.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	settings, err := s.db.ListSettings(ctx)
	if err != nil {
		s.logger.Error("list_settings_failed", "error", err, "operation", "get_settings", "component", "handler")
		writeError(w, r, fmt.Errorf("failed to list settings"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Filter out sensitive keys.
	filtered := make(map[string]string, len(settings))
	for k, v := range settings {
		if !sensitiveSettings[k] {
			filtered[k] = v
		}
	}

	writeJSON(w, http.StatusOK, filtered)
}

// handleUpdateSettings updates whitelisted settings.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req map[string]string
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	// Validate all keys are allowed.
	for k := range req {
		if !allowedSettings[k] {
			writeError(w, r, fmt.Errorf("setting %q is not allowed", k), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
			return
		}
	}

	// Save each setting.
	for k, v := range req {
		if err := s.db.SetSetting(ctx, k, v); err != nil {
			s.logger.Error("set_setting_failed", "error", err, "key", k, "operation", "update_settings", "component", "handler")
			writeError(w, r, fmt.Errorf("failed to save setting %q", k), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	s.logger.Info("settings_updated", "keys_count", len(req), "component", "handler")
	s.auditf(r, "settings.updated", "settings", "updated %d setting(s)", len(req))

	// Return updated settings.
	s.handleGetSettings(w, r)
}
