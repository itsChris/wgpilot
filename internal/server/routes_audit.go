package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
)

type auditLogResponse struct {
	Entries []auditEntryJSON `json:"entries"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

type auditEntryJSON struct {
	ID        int64  `json:"id"`
	Timestamp int64  `json:"timestamp"`
	UserID    int64  `json:"user_id"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
	Detail    string `json:"detail"`
	IPAddress string `json:"ip_address"`
}

// handleAuditLog returns a paginated list of audit log entries.
// Query params: limit (default 50, max 200), offset (default 0), action, resource.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	filter := db.AuditFilter{
		Action:   q.Get("action"),
		Resource: q.Get("resource"),
	}

	entries, total, err := s.db.ListAuditLog(ctx, limit, offset, filter)
	if err != nil {
		s.logger.Error("list_audit_log_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("failed to list audit log"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	resp := auditLogResponse{
		Entries: make([]auditEntryJSON, 0, len(entries)),
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, auditEntryJSON{
			ID:        e.ID,
			Timestamp: e.Timestamp.Unix(),
			UserID:    e.UserID,
			Action:    e.Action,
			Resource:  e.Resource,
			Detail:    e.Detail,
			IPAddress: e.IPAddress,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
