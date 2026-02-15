package server

import (
	"fmt"
	"net/http"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	User userInfo `json:"user"`
}

type userInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type setupRequest struct {
	OTP      string `json:"otp"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin authenticates a user and issues a session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if !s.rateLimiter.Allow(ip) {
		s.logger.Warn("auth_rate_limited",
			"remote_addr", ip,
			"component", "auth",
		)
		w.Header().Set("Retry-After", "60")
		writeError(w, r, fmt.Errorf("too many login attempts"), apperr.ErrRateLimited, http.StatusTooManyRequests, s.devMode)
		return
	}

	var req loginRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, r, fmt.Errorf("username and password required"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	user, err := s.db.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		s.logger.Error("auth_login_db_error",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if user == nil {
		s.logger.Warn("auth_login_failed",
			"user", req.Username,
			"remote_addr", ip,
			"reason", "user_not_found",
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("invalid credentials"), apperr.ErrInvalidCredentials, http.StatusUnauthorized, s.devMode)
		return
	}

	if err := auth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		s.logger.Warn("auth_login_failed",
			"user", req.Username,
			"remote_addr", ip,
			"reason", "invalid_password",
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("invalid credentials"), apperr.ErrInvalidCredentials, http.StatusUnauthorized, s.devMode)
		return
	}

	token, err := s.jwtService.Generate(user.ID, user.Username, user.Role)
	if err != nil {
		s.logger.Error("auth_token_generation_failed",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.sessions.SetCookie(w, token, int(s.jwtService.TTL().Seconds()))

	s.logger.Info("auth_login_success",
		"user", user.Username,
		"remote_addr", ip,
		"component", "auth",
	)

	writeJSON(w, http.StatusOK, loginResponse{
		User: userInfo{ID: user.ID, Username: user.Username},
	})
}

// handleSetup handles the first-run OTP setup flow.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complete, err := s.db.GetSetting(ctx, "setup_complete")
	if err != nil {
		s.logger.Error("setup_check_failed",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if complete == "true" {
		writeError(w, r, fmt.Errorf("setup already completed"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}

	var req setupRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if req.OTP == "" || req.Username == "" || req.Password == "" {
		writeError(w, r, fmt.Errorf("otp, username, and password required"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	if len(req.Password) < auth.MinPasswordLength {
		writeError(w, r, fmt.Errorf("password must be at least 10 characters"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	otpHash, err := s.db.GetSetting(ctx, "setup_otp")
	if err != nil {
		s.logger.Error("setup_otp_read_failed",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if otpHash == "" {
		writeError(w, r, fmt.Errorf("setup already completed"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}

	if err := auth.VerifyPassword(otpHash, req.OTP); err != nil {
		s.logger.Warn("setup_invalid_otp",
			"remote_addr", r.RemoteAddr,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("invalid setup password"), apperr.ErrInvalidOTP, http.StatusUnauthorized, s.devMode)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("setup_hash_password_failed",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	userID, err := s.db.CreateUser(ctx, &db.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil {
		s.logger.Error("setup_create_user_failed",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	if err := s.db.DeleteSetting(ctx, "setup_otp"); err != nil {
		s.logger.Error("setup_delete_otp_failed",
			"error", err,
			"component", "auth",
		)
	}
	if err := s.db.SetSetting(ctx, "setup_complete", "true"); err != nil {
		s.logger.Error("setup_set_complete_failed",
			"error", err,
			"component", "auth",
		)
	}

	token, err := s.jwtService.Generate(userID, req.Username, "admin")
	if err != nil {
		s.logger.Error("setup_token_generation_failed",
			"error", err,
			"component", "auth",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.sessions.SetCookie(w, token, int(s.jwtService.TTL().Seconds()))

	s.logger.Info("setup_completed",
		"user", req.Username,
		"user_id", userID,
		"remote_addr", r.RemoteAddr,
		"component", "auth",
	)

	writeJSON(w, http.StatusCreated, loginResponse{
		User: userInfo{ID: userID, Username: req.Username},
	})
}

// handleLogout clears the session cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.ClearCookie(w)

	s.logger.Info("auth_logout",
		"remote_addr", r.RemoteAddr,
		"component", "auth",
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// handleMe returns the current authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := auth.UserFromContext(r.Context())
	if claims == nil {
		writeError(w, r, fmt.Errorf("unauthorized"), apperr.ErrUnauthorized, http.StatusUnauthorized, s.devMode)
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
