package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// APIKey represents a row in the api_keys table.
type APIKey struct {
	ID        int64
	Name      string
	KeyHash   string
	KeyPrefix string
	UserID    int64
	Role      string
	ExpiresAt *time.Time
	CreatedAt time.Time
	LastUsed  *time.Time
}

// CreateAPIKey inserts a new API key and returns its ID.
func (d *DB) CreateAPIKey(ctx context.Context, k *APIKey) (int64, error) {
	var expiresAt *int64
	if k.ExpiresAt != nil {
		ts := k.ExpiresAt.Unix()
		expiresAt = &ts
	}

	result, err := d.ExecContext(ctx, `
		INSERT INTO api_keys (name, key_hash, key_prefix, user_id, role, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		k.Name, k.KeyHash, k.KeyPrefix, k.UserID, k.Role, expiresAt,
	)
	if err != nil {
		return 0, fmt.Errorf("db: create api key: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db: create api key last insert id: %w", err)
	}
	return id, nil
}

// GetAPIKeyByHash retrieves an API key by its hash.
// Returns nil, nil if not found.
func (d *DB) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	k := &APIKey{}
	var createdAt int64
	var expiresAt, lastUsed sql.NullInt64
	err := d.QueryRowContext(ctx, `
		SELECT id, name, key_hash, key_prefix, user_id, role, expires_at, created_at, last_used
		FROM api_keys WHERE key_hash = ?`, hash,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.UserID, &k.Role, &expiresAt, &createdAt, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get api key by hash: %w", err)
	}
	k.CreatedAt = time.Unix(createdAt, 0)
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		k.ExpiresAt = &t
	}
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0)
		k.LastUsed = &t
	}
	return k, nil
}

// ListAPIKeys returns all API keys for a user.
func (d *DB) ListAPIKeys(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, name, key_hash, key_prefix, user_id, role, expires_at, created_at, last_used
		FROM api_keys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("db: list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var createdAt int64
		var expiresAt, lastUsed sql.NullInt64
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.UserID, &k.Role, &expiresAt, &createdAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("db: scan api key: %w", err)
		}
		k.CreatedAt = time.Unix(createdAt, 0)
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			k.ExpiresAt = &t
		}
		if lastUsed.Valid {
			t := time.Unix(lastUsed.Int64, 0)
			k.LastUsed = &t
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// ListAllAPIKeys returns all API keys.
func (d *DB) ListAllAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, name, key_hash, key_prefix, user_id, role, expires_at, created_at, last_used
		FROM api_keys ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("db: list all api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var createdAt int64
		var expiresAt, lastUsed sql.NullInt64
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.UserID, &k.Role, &expiresAt, &createdAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("db: scan api key: %w", err)
		}
		k.CreatedAt = time.Unix(createdAt, 0)
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			k.ExpiresAt = &t
		}
		if lastUsed.Valid {
			t := time.Unix(lastUsed.Int64, 0)
			k.LastUsed = &t
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// UpdateAPIKeyLastUsed updates the last_used timestamp.
func (d *DB) UpdateAPIKeyLastUsed(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `
		UPDATE api_keys SET last_used = unixepoch() WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("db: update api key last used %d: %w", id, err)
	}
	return nil
}

// DeleteAPIKey deletes an API key by ID.
func (d *DB) DeleteAPIKey(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("db: delete api key %d: %w", id, err)
	}
	return nil
}
