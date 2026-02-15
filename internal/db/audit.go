package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AuditEntry represents a row in the audit_log table.
type AuditEntry struct {
	ID        int64
	Timestamp time.Time
	UserID    int64
	Action    string
	Resource  string
	Detail    string
	IPAddress string
}

// AuditFilter holds optional filters for listing audit entries.
type AuditFilter struct {
	Action   string
	Resource string
	UserID   int64
}

// InsertAuditEntry inserts a new audit log entry.
func (d *DB) InsertAuditEntry(ctx context.Context, entry *AuditEntry) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO audit_log (user_id, action, resource, detail, ip_address)
		VALUES (?, ?, ?, ?, ?)`,
		entry.UserID, entry.Action, entry.Resource, entry.Detail, entry.IPAddress,
	)
	if err != nil {
		return fmt.Errorf("db: insert audit entry: %w", err)
	}
	return nil
}

// ListAuditLog returns audit entries matching the given filters, with pagination.
// Returns the entries and the total count (for pagination).
func (d *DB) ListAuditLog(ctx context.Context, limit, offset int, filter AuditFilter) ([]AuditEntry, int, error) {
	var where []string
	var args []any

	if filter.Action != "" {
		where = append(where, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Resource != "" {
		where = append(where, "resource = ?")
		args = append(args, filter.Resource)
	}
	if filter.UserID > 0 {
		where = append(where, "user_id = ?")
		args = append(args, filter.UserID)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Get total count.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", whereClause)
	var total int
	if err := d.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("db: count audit log: %w", err)
	}

	// Get page of results.
	query := fmt.Sprintf(`
		SELECT id, timestamp, user_id, action, resource, detail, ip_address
		FROM audit_log %s
		ORDER BY id DESC
		LIMIT ? OFFSET ?`, whereClause)

	pageArgs := append(args, limit, offset)
	rows, err := d.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("db: list audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.UserID, &e.Action, &e.Resource, &e.Detail, &e.IPAddress); err != nil {
			return nil, 0, fmt.Errorf("db: scan audit entry: %w", err)
		}
		e.Timestamp = time.Unix(ts, 0)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("db: iterate audit log: %w", err)
	}

	return entries, total, nil
}
