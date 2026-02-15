package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetSetting retrieves a setting value by key.
// Returns empty string and nil error if the key does not exist.
func (d *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := d.QueryRowContext(ctx,
		"SELECT value FROM settings WHERE key = ?", key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("db: get setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting upserts a setting key-value pair.
func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.ExecContext(ctx,
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("db: set setting %q: %w", key, err)
	}
	return nil
}

// DeleteSetting removes a setting by key.
func (d *DB) DeleteSetting(ctx context.Context, key string) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM settings WHERE key = ?", key,
	)
	if err != nil {
		return fmt.Errorf("db: delete setting %q: %w", key, err)
	}
	return nil
}

// ListSettings returns all settings as a map.
func (d *DB) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := d.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, fmt.Errorf("db: list settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("db: scan setting: %w", err)
		}
		settings[k] = v
	}
	return settings, rows.Err()
}
