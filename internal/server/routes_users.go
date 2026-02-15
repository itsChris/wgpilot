package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
)

// ── Request/Response types ───────────────────────────────────────────

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type userResponse struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ── Handlers ─────────────────────────────────────────────────────────

// handleListUsers returns all users (admin only).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.logger.Error("list_users_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("failed to list users"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	result := make([]userResponse, 0, len(users))
	for _, u := range users {
		result = append(result, userToResponse(&u))
	}

	writeJSON(w, http.StatusOK, result)
}

// handleCreateUser creates a new user (admin only).
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createUserRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if req.Username == "" {
		writeError(w, r, fmt.Errorf("username is required"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}
	if req.Password == "" || len(req.Password) < auth.MinPasswordLength {
		writeError(w, r, fmt.Errorf("password must be at least %d characters", auth.MinPasswordLength), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}
	if req.Role == "" {
		req.Role = "viewer"
	}
	if req.Role != "admin" && req.Role != "viewer" {
		writeError(w, r, fmt.Errorf("role must be admin or viewer"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	// Check for duplicate username.
	existing, err := s.db.GetUserByUsername(ctx, req.Username)
	if err != nil {
		s.logger.Error("get_user_by_username_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if existing != nil {
		writeError(w, r, fmt.Errorf("username %q already exists", req.Username), apperr.ErrValidation, http.StatusConflict, s.devMode)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("hash_password_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	id, err := s.db.CreateUser(ctx, &db.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
	})
	if err != nil {
		s.logger.Error("create_user_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("failed to create user"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	created, err := s.db.GetUserByID(ctx, id)
	if err != nil || created == nil {
		s.logger.Error("get_created_user_failed", "error", err, "component", "handler", "user_id", id)
		writeError(w, r, fmt.Errorf("failed to retrieve created user"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("user_created", "user_id", id, "username", req.Username, "role", req.Role, "component", "handler")
	s.auditf(r, "user.created", "user", "created user %q (id=%d, role=%s)", req.Username, id, req.Role)

	writeJSON(w, http.StatusCreated, userToResponse(created))
}

// handleDeleteUser deletes a user (admin only). Cannot delete self.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid user ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	// Prevent self-deletion.
	claims := auth.UserFromContext(ctx)
	if claims != nil {
		var callerID int64
		if _, parseErr := fmt.Sscanf(claims.Subject, "%d", &callerID); parseErr == nil && callerID == id {
			writeError(w, r, fmt.Errorf("cannot delete your own account"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
			return
		}
	}

	user, err := s.db.GetUserByID(ctx, id)
	if err != nil {
		s.logger.Error("get_user_failed", "error", err, "component", "handler", "user_id", id)
		writeError(w, r, fmt.Errorf("failed to get user"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if user == nil {
		writeError(w, r, fmt.Errorf("user %d not found", id), apperr.ErrValidation, http.StatusNotFound, s.devMode)
		return
	}

	if err := s.db.DeleteUser(ctx, id); err != nil {
		s.logger.Error("delete_user_failed", "error", err, "component", "handler", "user_id", id)
		writeError(w, r, fmt.Errorf("failed to delete user"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("user_deleted", "user_id", id, "username", user.Username, "component", "handler")
	s.auditf(r, "user.deleted", "user", "deleted user %q (id=%d)", user.Username, id)

	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ──────────────────────────────────────────────────────────

func userToResponse(u *db.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		CreatedAt: u.CreatedAt.Unix(),
		UpdatedAt: u.UpdatedAt.Unix(),
	}
}
