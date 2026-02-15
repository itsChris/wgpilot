package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User represents a row in the users table.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreateUser inserts a new user and returns its ID.
func (d *DB) CreateUser(ctx context.Context, u *User) (int64, error) {
	result, err := d.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role)
		VALUES (?, ?, ?)`,
		u.Username, u.PasswordHash, u.Role,
	)
	if err != nil {
		return 0, fmt.Errorf("db: create user %q: %w", u.Username, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db: create user last insert id: %w", err)
	}
	return id, nil
}

// GetUserByUsername retrieves a user by username.
// Returns nil, nil if the user does not exist.
func (d *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{}
	var createdAt, updatedAt int64
	err := d.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get user by username %q: %w", username, err)
	}
	u.CreatedAt = time.Unix(createdAt, 0)
	u.UpdatedAt = time.Unix(updatedAt, 0)
	return u, nil
}

// UpdateUserPassword updates a user's password hash.
func (d *DB) UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := d.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = unixepoch()
		WHERE id = ?`, passwordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("db: update user %d password: %w", userID, err)
	}
	return nil
}

// ListUsers returns all users.
func (d *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("db: list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var createdAt, updatedAt int64
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("db: scan user: %w", err)
		}
		u.CreatedAt = time.Unix(createdAt, 0)
		u.UpdatedAt = time.Unix(updatedAt, 0)
		users = append(users, u)
	}
	return users, rows.Err()
}

// DeleteUser deletes a user by ID.
func (d *DB) DeleteUser(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("db: delete user %d: %w", id, err)
	}
	return nil
}

// GetUserByID retrieves a user by ID.
// Returns nil, nil if the user does not exist.
func (d *DB) GetUserByID(ctx context.Context, id int64) (*User, error) {
	u := &User{}
	var createdAt, updatedAt int64
	err := d.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get user by id %d: %w", id, err)
	}
	u.CreatedAt = time.Unix(createdAt, 0)
	u.UpdatedAt = time.Unix(updatedAt, 0)
	return u, nil
}
