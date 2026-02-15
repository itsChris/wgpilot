package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Peer represents a row in the peers table.
type Peer struct {
	ID                  int64
	NetworkID           int64
	Name                string
	Email               string
	PrivateKey          string
	PublicKey           string
	PresharedKey        string
	AllowedIPs          string
	Endpoint            string
	PersistentKeepalive int
	Role                string
	SiteNetworks        string
	Enabled             bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// CreatePeer inserts a new peer and returns its ID.
func (d *DB) CreatePeer(ctx context.Context, p *Peer) (int64, error) {
	result, err := d.ExecContext(ctx, `
		INSERT INTO peers (network_id, name, email, private_key, public_key, preshared_key,
		                   allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.NetworkID, p.Name, p.Email, p.PrivateKey, p.PublicKey, p.PresharedKey,
		p.AllowedIPs, p.Endpoint, p.PersistentKeepalive,
		p.Role, p.SiteNetworks, p.Enabled,
	)
	if err != nil {
		return 0, fmt.Errorf("db: create peer %q: %w", p.Name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db: create peer last insert id: %w", err)
	}
	return id, nil
}

// GetPeerByID retrieves a peer by ID.
func (d *DB) GetPeerByID(ctx context.Context, id int64) (*Peer, error) {
	p := &Peer{}
	var createdAt, updatedAt int64
	err := d.QueryRowContext(ctx, `
		SELECT id, network_id, name, email, private_key, public_key, preshared_key,
		       allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled,
		       created_at, updated_at
		FROM peers WHERE id = ?`, id,
	).Scan(
		&p.ID, &p.NetworkID, &p.Name, &p.Email, &p.PrivateKey, &p.PublicKey, &p.PresharedKey,
		&p.AllowedIPs, &p.Endpoint, &p.PersistentKeepalive,
		&p.Role, &p.SiteNetworks, &p.Enabled,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get peer %d: %w", id, err)
	}
	p.CreatedAt = time.Unix(createdAt, 0)
	p.UpdatedAt = time.Unix(updatedAt, 0)
	return p, nil
}

// ListPeersByNetworkID returns all peers for a given network.
func (d *DB) ListPeersByNetworkID(ctx context.Context, networkID int64) ([]Peer, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, network_id, name, email, private_key, public_key, preshared_key,
		       allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled,
		       created_at, updated_at
		FROM peers WHERE network_id = ? ORDER BY id`, networkID,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list peers for network %d: %w", networkID, err)
	}
	defer rows.Close()

	var peers []Peer
	for rows.Next() {
		var p Peer
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&p.ID, &p.NetworkID, &p.Name, &p.Email, &p.PrivateKey, &p.PublicKey, &p.PresharedKey,
			&p.AllowedIPs, &p.Endpoint, &p.PersistentKeepalive,
			&p.Role, &p.SiteNetworks, &p.Enabled,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan peer: %w", err)
		}
		p.CreatedAt = time.Unix(createdAt, 0)
		p.UpdatedAt = time.Unix(updatedAt, 0)
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

// UpdatePeer updates a peer's mutable fields.
func (d *DB) UpdatePeer(ctx context.Context, p *Peer) error {
	_, err := d.ExecContext(ctx, `
		UPDATE peers SET
			name = ?, email = ?, private_key = ?, public_key = ?, preshared_key = ?,
			allowed_ips = ?, endpoint = ?, persistent_keepalive = ?,
			role = ?, site_networks = ?, enabled = ?,
			updated_at = unixepoch()
		WHERE id = ?`,
		p.Name, p.Email, p.PrivateKey, p.PublicKey, p.PresharedKey,
		p.AllowedIPs, p.Endpoint, p.PersistentKeepalive,
		p.Role, p.SiteNetworks, p.Enabled,
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("db: update peer %d: %w", p.ID, err)
	}
	return nil
}

// DeletePeer deletes a peer by ID.
func (d *DB) DeletePeer(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM peers WHERE id = ?", id,
	)
	if err != nil {
		return fmt.Errorf("db: delete peer %d: %w", id, err)
	}
	return nil
}
