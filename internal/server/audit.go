package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
)

// auditLog records an action in the audit log. It extracts the user ID from
// the request context and the IP address from the request. Errors are logged
// but do not affect the request outcome.
func (s *Server) auditLog(r *http.Request, action, resource, detail string) {
	ctx := r.Context()

	var userID int64
	if claims := auth.UserFromContext(ctx); claims != nil {
		if id, err := strconv.ParseInt(claims.Subject, 10, 64); err == nil {
			userID = id
		}
	}

	entry := &db.AuditEntry{
		UserID:    userID,
		Action:    action,
		Resource:  resource,
		Detail:    detail,
		IPAddress: r.RemoteAddr,
	}

	if err := s.db.InsertAuditEntry(ctx, entry); err != nil {
		s.logger.Error("audit_log_insert_failed",
			"error", err,
			"action", action,
			"resource", resource,
			"component", "audit",
		)
	}
}

// auditf is a convenience wrapper that formats the detail string.
func (s *Server) auditf(r *http.Request, action, resource, format string, args ...any) {
	s.auditLog(r, action, resource, fmt.Sprintf(format, args...))
}
