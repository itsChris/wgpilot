package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
)

// ── Request/Response types ───────────────────────────────────────────

type createAlertRequest struct {
	Type      string `json:"type"`
	Threshold string `json:"threshold"`
	Notify    string `json:"notify"`
	Enabled   *bool  `json:"enabled"`
}

type updateAlertRequest struct {
	Type      *string `json:"type"`
	Threshold *string `json:"threshold"`
	Notify    *string `json:"notify"`
	Enabled   *bool   `json:"enabled"`
}

type alertResponse struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Threshold string `json:"threshold"`
	Notify    string `json:"notify"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
}

// ── Validation ───────────────────────────────────────────────────────

var validAlertTypes = map[string]bool{
	"peer_offline":    true,
	"high_latency":    true,
	"bandwidth_limit": true,
}

var validNotifyMethods = map[string]bool{
	"email": true,
	"log":   true,
}

// ── Handlers ─────────────────────────────────────────────────────────

// handleListAlerts returns all alerts.
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	alerts, err := s.db.ListAlerts(ctx)
	if err != nil {
		s.logger.Error("list_alerts_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("failed to list alerts"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	result := make([]alertResponse, 0, len(alerts))
	for _, a := range alerts {
		result = append(result, alertToResponse(&a))
	}

	writeJSON(w, http.StatusOK, result)
}

// handleCreateAlert creates a new alert.
func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createAlertRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	// Validate type.
	if !validAlertTypes[req.Type] {
		writeError(w, r, fmt.Errorf("invalid alert type %q", req.Type), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	if req.Threshold == "" {
		writeError(w, r, fmt.Errorf("threshold is required"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	notify := "email"
	if req.Notify != "" {
		if !validNotifyMethods[req.Notify] {
			writeError(w, r, fmt.Errorf("invalid notify method %q", req.Notify), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
			return
		}
		notify = req.Notify
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	alert := &db.Alert{
		Type:      req.Type,
		Threshold: req.Threshold,
		Notify:    notify,
		Enabled:   enabled,
	}

	id, err := s.db.CreateAlert(ctx, alert)
	if err != nil {
		s.logger.Error("create_alert_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("failed to create alert"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	created, err := s.db.GetAlertByID(ctx, id)
	if err != nil || created == nil {
		s.logger.Error("get_created_alert_failed", "error", err, "component", "handler", "alert_id", id)
		writeError(w, r, fmt.Errorf("failed to retrieve created alert"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("alert_created", "alert_id", id, "type", req.Type, "component", "handler")
	s.auditf(r, "alert.created", "alert", "created alert (id=%d, type=%s)", id, req.Type)

	writeJSON(w, http.StatusCreated, alertToResponse(created))
}

// handleUpdateAlert updates an alert's fields.
func (s *Server) handleUpdateAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid alert ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	alert, err := s.db.GetAlertByID(ctx, id)
	if err != nil {
		s.logger.Error("get_alert_failed", "error", err, "component", "handler", "alert_id", id)
		writeError(w, r, fmt.Errorf("failed to get alert"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if alert == nil {
		writeError(w, r, fmt.Errorf("alert %d not found", id), apperr.ErrAlertNotFound, http.StatusNotFound, s.devMode)
		return
	}

	var req updateAlertRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if req.Type != nil {
		if !validAlertTypes[*req.Type] {
			writeError(w, r, fmt.Errorf("invalid alert type %q", *req.Type), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
			return
		}
		alert.Type = *req.Type
	}
	if req.Threshold != nil {
		alert.Threshold = *req.Threshold
	}
	if req.Notify != nil {
		if !validNotifyMethods[*req.Notify] {
			writeError(w, r, fmt.Errorf("invalid notify method %q", *req.Notify), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
			return
		}
		alert.Notify = *req.Notify
	}
	if req.Enabled != nil {
		alert.Enabled = *req.Enabled
	}

	if err := s.db.UpdateAlert(ctx, alert); err != nil {
		s.logger.Error("update_alert_failed", "error", err, "component", "handler", "alert_id", id)
		writeError(w, r, fmt.Errorf("failed to update alert"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	updated, _ := s.db.GetAlertByID(ctx, id)
	if updated == nil {
		updated = alert
	}

	s.logger.Info("alert_updated", "alert_id", id, "component", "handler")
	s.auditf(r, "alert.updated", "alert", "updated alert (id=%d)", id)

	writeJSON(w, http.StatusOK, alertToResponse(updated))
}

// handleDeleteAlert deletes an alert.
func (s *Server) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid alert ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	alert, err := s.db.GetAlertByID(ctx, id)
	if err != nil {
		s.logger.Error("get_alert_failed", "error", err, "component", "handler", "alert_id", id)
		writeError(w, r, fmt.Errorf("failed to get alert"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if alert == nil {
		writeError(w, r, fmt.Errorf("alert %d not found", id), apperr.ErrAlertNotFound, http.StatusNotFound, s.devMode)
		return
	}

	if err := s.db.DeleteAlert(ctx, id); err != nil {
		s.logger.Error("delete_alert_failed", "error", err, "component", "handler", "alert_id", id)
		writeError(w, r, fmt.Errorf("failed to delete alert"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("alert_deleted", "alert_id", id, "type", alert.Type, "component", "handler")
	s.auditf(r, "alert.deleted", "alert", "deleted alert (id=%d, type=%s)", id, alert.Type)

	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ──────────────────────────────────────────────────────────

func alertToResponse(a *db.Alert) alertResponse {
	return alertResponse{
		ID:        a.ID,
		Type:      a.Type,
		Threshold: a.Threshold,
		Notify:    a.Notify,
		Enabled:   a.Enabled,
		CreatedAt: a.CreatedAt.Unix(),
	}
}
