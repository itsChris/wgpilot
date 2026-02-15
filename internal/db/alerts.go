package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Alert represents a row in the alerts table.
type Alert struct {
	ID        int64
	Type      string
	Threshold string
	Notify    string
	Enabled   bool
	CreatedAt time.Time
}

// CreateAlert inserts a new alert and returns its ID.
func (d *DB) CreateAlert(ctx context.Context, a *Alert) (int64, error) {
	result, err := d.ExecContext(ctx, `
		INSERT INTO alerts (type, threshold, notify, enabled)
		VALUES (?, ?, ?, ?)`,
		a.Type, a.Threshold, a.Notify, a.Enabled,
	)
	if err != nil {
		return 0, fmt.Errorf("db: create alert: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db: create alert last insert id: %w", err)
	}
	return id, nil
}

// GetAlertByID retrieves an alert by ID.
// Returns nil, nil if not found.
func (d *DB) GetAlertByID(ctx context.Context, id int64) (*Alert, error) {
	a := &Alert{}
	var createdAt int64
	err := d.QueryRowContext(ctx, `
		SELECT id, type, threshold, notify, enabled, created_at
		FROM alerts WHERE id = ?`, id,
	).Scan(&a.ID, &a.Type, &a.Threshold, &a.Notify, &a.Enabled, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get alert %d: %w", id, err)
	}
	a.CreatedAt = time.Unix(createdAt, 0)
	return a, nil
}

// ListAlerts returns all alerts.
func (d *DB) ListAlerts(ctx context.Context) ([]Alert, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, type, threshold, notify, enabled, created_at
		FROM alerts ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("db: list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var createdAt int64
		if err := rows.Scan(&a.ID, &a.Type, &a.Threshold, &a.Notify, &a.Enabled, &createdAt); err != nil {
			return nil, fmt.Errorf("db: scan alert: %w", err)
		}
		a.CreatedAt = time.Unix(createdAt, 0)
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// ListEnabledAlerts returns all enabled alerts.
func (d *DB) ListEnabledAlerts(ctx context.Context) ([]Alert, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, type, threshold, notify, enabled, created_at
		FROM alerts WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("db: list enabled alerts: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var createdAt int64
		if err := rows.Scan(&a.ID, &a.Type, &a.Threshold, &a.Notify, &a.Enabled, &createdAt); err != nil {
			return nil, fmt.Errorf("db: scan alert: %w", err)
		}
		a.CreatedAt = time.Unix(createdAt, 0)
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// UpdateAlert updates an alert's mutable fields.
func (d *DB) UpdateAlert(ctx context.Context, a *Alert) error {
	_, err := d.ExecContext(ctx, `
		UPDATE alerts SET type = ?, threshold = ?, notify = ?, enabled = ?
		WHERE id = ?`,
		a.Type, a.Threshold, a.Notify, a.Enabled, a.ID,
	)
	if err != nil {
		return fmt.Errorf("db: update alert %d: %w", a.ID, err)
	}
	return nil
}

// DeleteAlert deletes an alert by ID.
func (d *DB) DeleteAlert(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM alerts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("db: delete alert %d: %w", id, err)
	}
	return nil
}
