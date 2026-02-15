package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Network represents a row in the networks table.
type Network struct {
	ID               int64
	Name             string
	Interface        string
	Mode             string
	Subnet           string
	ListenPort       int
	PrivateKey       string
	PublicKey        string
	DNSServers       string
	NATEnabled       bool
	InterPeerRouting bool
	Enabled          bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreateNetwork inserts a new network and returns its ID.
func (d *DB) CreateNetwork(ctx context.Context, n *Network) (int64, error) {
	result, err := d.ExecContext(ctx, `
		INSERT INTO networks (name, interface, mode, subnet, listen_port, private_key, public_key, dns_servers, nat_enabled, inter_peer_routing, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Name, n.Interface, n.Mode, n.Subnet, n.ListenPort,
		n.PrivateKey, n.PublicKey, n.DNSServers,
		n.NATEnabled, n.InterPeerRouting, n.Enabled,
	)
	if err != nil {
		return 0, fmt.Errorf("db: create network %q: %w", n.Name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db: create network last insert id: %w", err)
	}
	return id, nil
}

// GetNetworkByID retrieves a network by ID.
func (d *DB) GetNetworkByID(ctx context.Context, id int64) (*Network, error) {
	n := &Network{}
	var createdAt, updatedAt int64
	err := d.QueryRowContext(ctx, `
		SELECT id, name, interface, mode, subnet, listen_port, private_key, public_key,
		       dns_servers, nat_enabled, inter_peer_routing, enabled, created_at, updated_at
		FROM networks WHERE id = ?`, id,
	).Scan(
		&n.ID, &n.Name, &n.Interface, &n.Mode, &n.Subnet, &n.ListenPort,
		&n.PrivateKey, &n.PublicKey, &n.DNSServers,
		&n.NATEnabled, &n.InterPeerRouting, &n.Enabled,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get network %d: %w", id, err)
	}
	n.CreatedAt = time.Unix(createdAt, 0)
	n.UpdatedAt = time.Unix(updatedAt, 0)
	return n, nil
}

// ListNetworks returns all networks.
func (d *DB) ListNetworks(ctx context.Context) ([]Network, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, name, interface, mode, subnet, listen_port, private_key, public_key,
		       dns_servers, nat_enabled, inter_peer_routing, enabled, created_at, updated_at
		FROM networks ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("db: list networks: %w", err)
	}
	defer rows.Close()

	var networks []Network
	for rows.Next() {
		var n Network
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&n.ID, &n.Name, &n.Interface, &n.Mode, &n.Subnet, &n.ListenPort,
			&n.PrivateKey, &n.PublicKey, &n.DNSServers,
			&n.NATEnabled, &n.InterPeerRouting, &n.Enabled,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan network: %w", err)
		}
		n.CreatedAt = time.Unix(createdAt, 0)
		n.UpdatedAt = time.Unix(updatedAt, 0)
		networks = append(networks, n)
	}
	return networks, rows.Err()
}

// UpdateNetwork updates a network's mutable fields.
func (d *DB) UpdateNetwork(ctx context.Context, n *Network) error {
	_, err := d.ExecContext(ctx, `
		UPDATE networks SET
			name = ?, mode = ?, subnet = ?, listen_port = ?,
			private_key = ?, public_key = ?, dns_servers = ?,
			nat_enabled = ?, inter_peer_routing = ?, enabled = ?,
			updated_at = unixepoch()
		WHERE id = ?`,
		n.Name, n.Mode, n.Subnet, n.ListenPort,
		n.PrivateKey, n.PublicKey, n.DNSServers,
		n.NATEnabled, n.InterPeerRouting, n.Enabled,
		n.ID,
	)
	if err != nil {
		return fmt.Errorf("db: update network %d: %w", n.ID, err)
	}
	return nil
}

// DeleteNetwork deletes a network by ID. Peers are cascade-deleted.
func (d *DB) DeleteNetwork(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM networks WHERE id = ?", id,
	)
	if err != nil {
		return fmt.Errorf("db: delete network %d: %w", id, err)
	}
	return nil
}
