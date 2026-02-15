package db

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/itsChris/wgpilot/internal/crypto"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all embedded SQL migration files against the database.
// Migrations are tracked in a _migrations table and only applied once.
func Migrate(ctx context.Context, d *DB, logger *slog.Logger) error {
	// Create migrations tracking table.
	_, err := d.conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _migrations (
			filename TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT (unixepoch())
		)
	`)
	if err != nil {
		return fmt.Errorf("db: create migrations table: %w", err)
	}

	// Read all migration files.
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("db: read migrations dir: %w", err)
	}

	// Sort by filename for deterministic ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Check if already applied.
		var count int
		err := d.conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM _migrations WHERE filename = ?",
			entry.Name(),
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("db: check migration %s: %w", entry.Name(), err)
		}
		if count > 0 {
			logger.Debug("migration_skipped",
				"filename", entry.Name(),
				"reason", "already_applied",
				"component", "db",
			)
			continue
		}

		// Read and execute the migration.
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("db: read migration %s: %w", entry.Name(), err)
		}

		sql := string(content)

		// Strip goose directives if present, only run the Up portion.
		sql = extractUpSection(sql)

		if _, err := d.conn.ExecContext(ctx, sql); err != nil {
			return fmt.Errorf("db: apply migration %s: %w", entry.Name(), err)
		}

		// Record it.
		if _, err := d.conn.ExecContext(ctx,
			"INSERT INTO _migrations (filename) VALUES (?)",
			entry.Name(),
		); err != nil {
			return fmt.Errorf("db: record migration %s: %w", entry.Name(), err)
		}

		logger.Info("migration_applied",
			"filename", entry.Name(),
			"component", "db",
		)
	}

	return nil
}

// MigrateEncryptKeys scans for unencrypted private keys in networks and peers,
// and encrypts them in-place. This is a data migration that runs after the
// encryption key is set.
func MigrateEncryptKeys(ctx context.Context, d *DB, logger *slog.Logger) error {
	if !d.encryptionKeySet {
		return nil
	}

	key := *d.encryptionKey

	// Encrypt network private keys.
	rows, err := d.conn.QueryContext(ctx, "SELECT id, private_key FROM networks WHERE private_key != ''")
	if err != nil {
		return fmt.Errorf("migrate encrypt: query networks: %w", err)
	}
	defer rows.Close()

	var networkUpdates []struct {
		id  int64
		enc string
	}
	for rows.Next() {
		var id int64
		var pk string
		if err := rows.Scan(&id, &pk); err != nil {
			return fmt.Errorf("migrate encrypt: scan network: %w", err)
		}
		if crypto.IsEncrypted(pk) {
			continue
		}
		enc, err := crypto.Encrypt(pk, key)
		if err != nil {
			return fmt.Errorf("migrate encrypt: encrypt network %d key: %w", id, err)
		}
		networkUpdates = append(networkUpdates, struct {
			id  int64
			enc string
		}{id, enc})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate encrypt: iterate networks: %w", err)
	}
	rows.Close()

	for _, u := range networkUpdates {
		if _, err := d.conn.ExecContext(ctx, "UPDATE networks SET private_key = ? WHERE id = ?", u.enc, u.id); err != nil {
			return fmt.Errorf("migrate encrypt: update network %d: %w", u.id, err)
		}
		logger.Info("key_encrypted", "table", "networks", "id", u.id, "component", "db")
	}

	// Encrypt peer private keys and preshared keys.
	peerRows, err := d.conn.QueryContext(ctx, "SELECT id, private_key, preshared_key FROM peers WHERE private_key != '' OR preshared_key != ''")
	if err != nil {
		return fmt.Errorf("migrate encrypt: query peers: %w", err)
	}
	defer peerRows.Close()

	var peerUpdates []struct {
		id   int64
		pk   string
		psk  string
	}
	for peerRows.Next() {
		var id int64
		var pk, psk string
		if err := peerRows.Scan(&id, &pk, &psk); err != nil {
			return fmt.Errorf("migrate encrypt: scan peer: %w", err)
		}
		encPK := pk
		if pk != "" && !crypto.IsEncrypted(pk) {
			enc, err := crypto.Encrypt(pk, key)
			if err != nil {
				return fmt.Errorf("migrate encrypt: encrypt peer %d private key: %w", id, err)
			}
			encPK = enc
		}
		encPSK := psk
		if psk != "" && !crypto.IsEncrypted(psk) {
			enc, err := crypto.Encrypt(psk, key)
			if err != nil {
				return fmt.Errorf("migrate encrypt: encrypt peer %d preshared key: %w", id, err)
			}
			encPSK = enc
		}
		if encPK != pk || encPSK != psk {
			peerUpdates = append(peerUpdates, struct {
				id  int64
				pk  string
				psk string
			}{id, encPK, encPSK})
		}
	}
	if err := peerRows.Err(); err != nil {
		return fmt.Errorf("migrate encrypt: iterate peers: %w", err)
	}
	peerRows.Close()

	for _, u := range peerUpdates {
		if _, err := d.conn.ExecContext(ctx, "UPDATE peers SET private_key = ?, preshared_key = ? WHERE id = ?", u.pk, u.psk, u.id); err != nil {
			return fmt.Errorf("migrate encrypt: update peer %d: %w", u.id, err)
		}
		logger.Info("key_encrypted", "table", "peers", "id", u.id, "component", "db")
	}

	total := len(networkUpdates) + len(peerUpdates)
	if total > 0 {
		logger.Info("key_encryption_migration_complete",
			"networks_encrypted", len(networkUpdates),
			"peers_encrypted", len(peerUpdates),
			"component", "db",
		)
	}

	return nil
}

// extractUpSection returns only the SQL between "-- +goose Up" and "-- +goose Down".
// If no goose directives are found, returns the full content.
func extractUpSection(sql string) string {
	upIdx := strings.Index(sql, "-- +goose Up")
	downIdx := strings.Index(sql, "-- +goose Down")

	if upIdx == -1 {
		return sql
	}

	start := upIdx + len("-- +goose Up")
	if downIdx == -1 {
		return strings.TrimSpace(sql[start:])
	}

	return strings.TrimSpace(sql[start:downIdx])
}
