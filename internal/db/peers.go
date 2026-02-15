package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/itsChris/wgpilot/internal/crypto"
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
	ExpiresAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// CreatePeer inserts a new peer and returns its ID.
// Private keys and preshared keys are encrypted at rest if an encryption key is set.
func (d *DB) CreatePeer(ctx context.Context, p *Peer) (int64, error) {
	privateKey := p.PrivateKey
	presharedKey := p.PresharedKey
	if d.encryptionKeySet {
		if privateKey != "" {
			enc, err := crypto.Encrypt(privateKey, *d.encryptionKey)
			if err != nil {
				return 0, fmt.Errorf("db: encrypt peer private key: %w", err)
			}
			privateKey = enc
		}
		if presharedKey != "" {
			enc, err := crypto.Encrypt(presharedKey, *d.encryptionKey)
			if err != nil {
				return 0, fmt.Errorf("db: encrypt peer preshared key: %w", err)
			}
			presharedKey = enc
		}
	}

	var expiresAt *int64
	if p.ExpiresAt != nil {
		ts := p.ExpiresAt.Unix()
		expiresAt = &ts
	}

	result, err := d.ExecContext(ctx, `
		INSERT INTO peers (network_id, name, email, private_key, public_key, preshared_key,
		                   allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.NetworkID, p.Name, p.Email, privateKey, p.PublicKey, presharedKey,
		p.AllowedIPs, p.Endpoint, p.PersistentKeepalive,
		p.Role, p.SiteNetworks, p.Enabled, expiresAt,
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
	var expiresAt sql.NullInt64
	err := d.QueryRowContext(ctx, `
		SELECT id, network_id, name, email, private_key, public_key, preshared_key,
		       allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled,
		       expires_at, created_at, updated_at
		FROM peers WHERE id = ?`, id,
	).Scan(
		&p.ID, &p.NetworkID, &p.Name, &p.Email, &p.PrivateKey, &p.PublicKey, &p.PresharedKey,
		&p.AllowedIPs, &p.Endpoint, &p.PersistentKeepalive,
		&p.Role, &p.SiteNetworks, &p.Enabled,
		&expiresAt, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get peer %d: %w", id, err)
	}
	p.CreatedAt = time.Unix(createdAt, 0)
	p.UpdatedAt = time.Unix(updatedAt, 0)
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		p.ExpiresAt = &t
	}
	if err := d.decryptPeerKeys(p); err != nil {
		return nil, fmt.Errorf("db: decrypt peer %d keys: %w", id, err)
	}
	return p, nil
}

// ListPeersByNetworkID returns all peers for a given network.
func (d *DB) ListPeersByNetworkID(ctx context.Context, networkID int64) ([]Peer, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, network_id, name, email, private_key, public_key, preshared_key,
		       allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled,
		       expires_at, created_at, updated_at
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
		var expiresAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.NetworkID, &p.Name, &p.Email, &p.PrivateKey, &p.PublicKey, &p.PresharedKey,
			&p.AllowedIPs, &p.Endpoint, &p.PersistentKeepalive,
			&p.Role, &p.SiteNetworks, &p.Enabled,
			&expiresAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan peer: %w", err)
		}
		p.CreatedAt = time.Unix(createdAt, 0)
		p.UpdatedAt = time.Unix(updatedAt, 0)
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			p.ExpiresAt = &t
		}
		if err := d.decryptPeerKeys(&p); err != nil {
			return nil, fmt.Errorf("db: decrypt peer %d keys: %w", p.ID, err)
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

// decryptPeerKeys decrypts private key and preshared key if encryption is enabled.
func (d *DB) decryptPeerKeys(p *Peer) error {
	if !d.encryptionKeySet {
		return nil
	}
	if p.PrivateKey != "" && crypto.IsEncrypted(p.PrivateKey) {
		plain, err := crypto.Decrypt(p.PrivateKey, *d.encryptionKey)
		if err != nil {
			return fmt.Errorf("decrypt private key: %w", err)
		}
		p.PrivateKey = plain
	}
	if p.PresharedKey != "" && crypto.IsEncrypted(p.PresharedKey) {
		plain, err := crypto.Decrypt(p.PresharedKey, *d.encryptionKey)
		if err != nil {
			return fmt.Errorf("decrypt preshared key: %w", err)
		}
		p.PresharedKey = plain
	}
	return nil
}

// UpdatePeer updates a peer's mutable fields.
func (d *DB) UpdatePeer(ctx context.Context, p *Peer) error {
	privateKey := p.PrivateKey
	presharedKey := p.PresharedKey
	if d.encryptionKeySet {
		if privateKey != "" && !crypto.IsEncrypted(privateKey) {
			enc, err := crypto.Encrypt(privateKey, *d.encryptionKey)
			if err != nil {
				return fmt.Errorf("db: encrypt peer %d private key: %w", p.ID, err)
			}
			privateKey = enc
		}
		if presharedKey != "" && !crypto.IsEncrypted(presharedKey) {
			enc, err := crypto.Encrypt(presharedKey, *d.encryptionKey)
			if err != nil {
				return fmt.Errorf("db: encrypt peer %d preshared key: %w", p.ID, err)
			}
			presharedKey = enc
		}
	}

	var expiresAt *int64
	if p.ExpiresAt != nil {
		ts := p.ExpiresAt.Unix()
		expiresAt = &ts
	}

	_, err := d.ExecContext(ctx, `
		UPDATE peers SET
			name = ?, email = ?, private_key = ?, public_key = ?, preshared_key = ?,
			allowed_ips = ?, endpoint = ?, persistent_keepalive = ?,
			role = ?, site_networks = ?, enabled = ?, expires_at = ?,
			updated_at = unixepoch()
		WHERE id = ?`,
		p.Name, p.Email, privateKey, p.PublicKey, presharedKey,
		p.AllowedIPs, p.Endpoint, p.PersistentKeepalive,
		p.Role, p.SiteNetworks, p.Enabled, expiresAt,
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("db: update peer %d: %w", p.ID, err)
	}
	return nil
}

// ListExpiredPeers returns all enabled peers whose expires_at is before now.
func (d *DB) ListExpiredPeers(ctx context.Context) ([]Peer, error) {
	now := time.Now().Unix()
	rows, err := d.QueryContext(ctx, `
		SELECT id, network_id, name, email, private_key, public_key, preshared_key,
		       allowed_ips, endpoint, persistent_keepalive, role, site_networks, enabled,
		       expires_at, created_at, updated_at
		FROM peers WHERE enabled = 1 AND expires_at IS NOT NULL AND expires_at < ? ORDER BY id`, now,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list expired peers: %w", err)
	}
	defer rows.Close()

	var peers []Peer
	for rows.Next() {
		var p Peer
		var createdAt, updatedAt int64
		var expiresAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.NetworkID, &p.Name, &p.Email, &p.PrivateKey, &p.PublicKey, &p.PresharedKey,
			&p.AllowedIPs, &p.Endpoint, &p.PersistentKeepalive,
			&p.Role, &p.SiteNetworks, &p.Enabled,
			&expiresAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan expired peer: %w", err)
		}
		p.CreatedAt = time.Unix(createdAt, 0)
		p.UpdatedAt = time.Unix(updatedAt, 0)
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			p.ExpiresAt = &t
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
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
