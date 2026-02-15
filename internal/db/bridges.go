package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Bridge represents a row in the network_bridges table.
type Bridge struct {
	ID          int64
	NetworkAID  int64
	NetworkBID  int64
	Direction   string
	AllowedCIDRs string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateBridge inserts a new bridge and returns its ID.
func (d *DB) CreateBridge(ctx context.Context, b *Bridge) (int64, error) {
	result, err := d.ExecContext(ctx, `
		INSERT INTO network_bridges (network_a_id, network_b_id, direction, allowed_cidrs, enabled)
		VALUES (?, ?, ?, ?, ?)`,
		b.NetworkAID, b.NetworkBID, b.Direction, b.AllowedCIDRs, b.Enabled,
	)
	if err != nil {
		return 0, fmt.Errorf("db: create bridge %d<->%d: %w", b.NetworkAID, b.NetworkBID, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db: create bridge last insert id: %w", err)
	}
	return id, nil
}

// GetBridgeByID retrieves a bridge by ID.
func (d *DB) GetBridgeByID(ctx context.Context, id int64) (*Bridge, error) {
	b := &Bridge{}
	var createdAt, updatedAt int64
	err := d.QueryRowContext(ctx, `
		SELECT id, network_a_id, network_b_id, direction, allowed_cidrs, enabled, created_at, updated_at
		FROM network_bridges WHERE id = ?`, id,
	).Scan(
		&b.ID, &b.NetworkAID, &b.NetworkBID, &b.Direction, &b.AllowedCIDRs,
		&b.Enabled, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get bridge %d: %w", id, err)
	}
	b.CreatedAt = time.Unix(createdAt, 0)
	b.UpdatedAt = time.Unix(updatedAt, 0)
	return b, nil
}

// ListBridges returns all bridges.
func (d *DB) ListBridges(ctx context.Context) ([]Bridge, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, network_a_id, network_b_id, direction, allowed_cidrs, enabled, created_at, updated_at
		FROM network_bridges ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("db: list bridges: %w", err)
	}
	defer rows.Close()

	var bridges []Bridge
	for rows.Next() {
		var b Bridge
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&b.ID, &b.NetworkAID, &b.NetworkBID, &b.Direction, &b.AllowedCIDRs,
			&b.Enabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan bridge: %w", err)
		}
		b.CreatedAt = time.Unix(createdAt, 0)
		b.UpdatedAt = time.Unix(updatedAt, 0)
		bridges = append(bridges, b)
	}
	return bridges, rows.Err()
}

// ListBridgesByNetworkID returns all bridges involving the given network.
func (d *DB) ListBridgesByNetworkID(ctx context.Context, networkID int64) ([]Bridge, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, network_a_id, network_b_id, direction, allowed_cidrs, enabled, created_at, updated_at
		FROM network_bridges
		WHERE network_a_id = ? OR network_b_id = ?
		ORDER BY id`, networkID, networkID)
	if err != nil {
		return nil, fmt.Errorf("db: list bridges for network %d: %w", networkID, err)
	}
	defer rows.Close()

	var bridges []Bridge
	for rows.Next() {
		var b Bridge
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&b.ID, &b.NetworkAID, &b.NetworkBID, &b.Direction, &b.AllowedCIDRs,
			&b.Enabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan bridge: %w", err)
		}
		b.CreatedAt = time.Unix(createdAt, 0)
		b.UpdatedAt = time.Unix(updatedAt, 0)
		bridges = append(bridges, b)
	}
	return bridges, rows.Err()
}

// BridgeExistsBetween checks if a bridge already exists between two networks
// (in either order).
func (d *DB) BridgeExistsBetween(ctx context.Context, networkAID, networkBID int64) (bool, error) {
	var count int
	err := d.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM network_bridges
		WHERE (network_a_id = ? AND network_b_id = ?)
		   OR (network_a_id = ? AND network_b_id = ?)`,
		networkAID, networkBID, networkBID, networkAID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("db: check bridge exists %d<->%d: %w", networkAID, networkBID, err)
	}
	return count > 0, nil
}

// DeleteBridge deletes a bridge by ID.
func (d *DB) DeleteBridge(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM network_bridges WHERE id = ?", id,
	)
	if err != nil {
		return fmt.Errorf("db: delete bridge %d: %w", id, err)
	}
	return nil
}
