package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
)

type createAPIKeyRequest struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	ExpiresIn string `json:"expires_in"` // e.g. "720h" for 30 days, empty for never
}

type createAPIKeyResponse struct {
	ID        int64   `json:"id"`
	Key       string  `json:"key"` // only returned on creation
	Name      string  `json:"name"`
	KeyPrefix string  `json:"key_prefix"`
	Role      string  `json:"role"`
	ExpiresAt *string `json:"expires_at"`
}

type apiKeyResponse struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	KeyPrefix string  `json:"key_prefix"`
	Role      string  `json:"role"`
	ExpiresAt *string `json:"expires_at"`
	CreatedAt string  `json:"created_at"`
	LastUsed  *string `json:"last_used"`
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	claims := auth.UserFromContext(r.Context())
	if claims == nil {
		writeError(w, r, fmt.Errorf("no user in context"), apperr.ErrUnauthorized, http.StatusUnauthorized, s.devMode)
		return
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("parse user id: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Admins see all keys, viewers see only their own.
	var keys []db.APIKey
	if claims.Role == "admin" {
		keys, err = s.db.ListAllAPIKeys(r.Context())
	} else {
		keys, err = s.db.ListAPIKeys(r.Context(), userID)
	}
	if err != nil {
		writeError(w, r, fmt.Errorf("list api keys: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	resp := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		item := apiKeyResponse{
			ID:        k.ID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			Role:      k.Role,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		}
		if k.ExpiresAt != nil {
			s := k.ExpiresAt.Format(time.RFC3339)
			item.ExpiresAt = &s
		}
		if k.LastUsed != nil {
			s := k.LastUsed.Format(time.RFC3339)
			item.LastUsed = &s
		}
		resp = append(resp, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	claims := auth.UserFromContext(r.Context())
	if claims == nil {
		writeError(w, r, fmt.Errorf("no user in context"), apperr.ErrUnauthorized, http.StatusUnauthorized, s.devMode)
		return
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("parse user id: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	var req createAPIKeyRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	// Validate.
	var fields []fieldError
	if req.Name == "" {
		fields = append(fields, fieldError{Field: "name", Message: "name is required"})
	}
	if req.Role == "" {
		req.Role = "admin"
	}
	if req.Role != "admin" && req.Role != "viewer" {
		fields = append(fields, fieldError{Field: "role", Message: "role must be 'admin' or 'viewer'"})
	}
	if len(fields) > 0 {
		writeValidationError(w, r, fields)
		return
	}

	// Parse expiry.
	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			writeValidationError(w, r, []fieldError{{Field: "expires_in", Message: "invalid duration format (e.g. '720h')"}})
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	// Generate key.
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		writeError(w, r, fmt.Errorf("generate api key: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	apiKey := &db.APIKey{
		Name:      req.Name,
		KeyHash:   hash,
		KeyPrefix: prefix,
		UserID:    userID,
		Role:      req.Role,
		ExpiresAt: expiresAt,
	}

	id, err := s.db.CreateAPIKey(r.Context(), apiKey)
	if err != nil {
		writeError(w, r, fmt.Errorf("create api key: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.auditf(r, "api_key.created", "api_key", "created API key %q (id=%d)", req.Name, id)

	resp := createAPIKeyResponse{
		ID:        id,
		Key:       key,
		Name:      req.Name,
		KeyPrefix: prefix,
		Role:      req.Role,
	}
	if expiresAt != nil {
		s := expiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &s
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid api key id"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	if err := s.db.DeleteAPIKey(r.Context(), id); err != nil {
		writeError(w, r, fmt.Errorf("delete api key: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.auditf(r, "api_key.deleted", "api_key", "deleted API key id=%d", id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
