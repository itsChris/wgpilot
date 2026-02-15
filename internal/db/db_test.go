package db

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// testDB creates an in-memory SQLite database with migrations applied.
func testDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()
	logger := slog.Default()

	d, err := New(ctx, ":memory:", logger, true)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	if err := Migrate(ctx, d, logger); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigration_AppliesCleanly(t *testing.T) {
	_ = testDB(t)
}

func TestMigration_Idempotent(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	d, err := New(ctx, ":memory:", logger, true)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Run migrations twice — should not error.
	if err := Migrate(ctx, d, logger); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	if err := Migrate(ctx, d, logger); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
}

func TestMigration_TablesExist(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	tables := []string{
		"settings", "users", "networks", "peers",
		"peer_snapshots", "network_bridges", "audit_log", "alerts",
	}
	for _, table := range tables {
		var name string
		err := d.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestDB_WALMode(t *testing.T) {
	// In-memory SQLite returns "memory" for journal_mode.
	// Use a temp file to verify WAL is properly set.
	ctx := context.Background()
	logger := slog.Default()
	tmpFile := t.TempDir() + "/test.db"

	d, err := New(ctx, tmpFile, logger, true)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	var mode string
	err = d.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("failed to check journal mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("expected WAL mode, got %q", mode)
	}
}

func TestDB_ForeignKeysEnabled(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	var fk int
	err := d.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("failed to check foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", fk)
	}
}

func TestDB_Transaction(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO settings (key, value) VALUES (?, ?)", "tx_key", "tx_val")
	if err != nil {
		t.Fatalf("tx exec: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("tx commit: %v", err)
	}

	val, err := d.GetSetting(ctx, "tx_key")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if val != "tx_val" {
		t.Errorf("expected %q, got %q", "tx_val", val)
	}
}

func TestDB_TransactionRollback(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO settings (key, value) VALUES (?, ?)", "rollback_key", "val")
	if err != nil {
		t.Fatalf("tx exec: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("tx rollback: %v", err)
	}

	val, err := d.GetSetting(ctx, "rollback_key")
	if err != nil {
		t.Fatalf("get setting after rollback: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty after rollback, got %q", val)
	}
}

// testNetwork creates a sample network for testing.
func testNetwork() *Network {
	return &Network{
		Name:             "Test Network",
		Interface:        "wg0",
		Mode:             "gateway",
		Subnet:           "10.0.0.0/24",
		ListenPort:       51820,
		PrivateKey:       "test-private-key",
		PublicKey:        "test-public-key",
		DNSServers:       "1.1.1.1,8.8.8.8",
		NATEnabled:       true,
		InterPeerRouting: false,
		Enabled:          true,
	}
}

// testPeer creates a sample peer for testing.
func testPeer(networkID int64) *Peer {
	return &Peer{
		NetworkID:           networkID,
		Name:                "Test Peer",
		Email:               "test@example.com",
		PrivateKey:          "peer-private-key",
		PublicKey:           "peer-public-key",
		PresharedKey:        "peer-psk",
		AllowedIPs:          "10.0.0.2/32",
		Endpoint:            "",
		PersistentKeepalive: 25,
		Role:                "client",
		SiteNetworks:        "",
		Enabled:             true,
	}
}

// testSnapshot creates a sample snapshot for testing.
func testSnapshot(peerID int64, ts time.Time) *PeerSnapshot {
	return &PeerSnapshot{
		PeerID:    peerID,
		Timestamp: ts,
		RxBytes:   1000,
		TxBytes:   2000,
		Online:    true,
	}
}
