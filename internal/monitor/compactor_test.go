package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/db"
)

func TestNewCompactor_NilStore(t *testing.T) {
	_, err := NewCompactor(nil, testLogger(), time.Hour, 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestNewCompactor_NilLogger(t *testing.T) {
	_, err := NewCompactor(&mockSnapshotStore{}, nil, time.Hour, 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

func TestCompactor_Compact_CallsStore(t *testing.T) {
	store := &mockSnapshotStore{}

	compactor, err := NewCompactor(store, testLogger(), time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}

	compactor.Compact(context.Background())

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.compacted != 1 {
		t.Fatalf("expected CompactSnapshots called once, got %d", store.compacted)
	}
}

func TestCompactor_Compact_WithRealDB(t *testing.T) {
	d := testDBForMonitor(t)
	ctx := context.Background()

	// Create network and peer.
	netID, err := d.CreateNetwork(ctx, &db.Network{
		Name: "Test", Interface: "wg0", Mode: "gateway",
		Subnet: "10.0.0.0/24", ListenPort: 51820,
		PrivateKey: "priv", PublicKey: "pub", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	peerID, err := d.CreatePeer(ctx, &db.Peer{
		NetworkID: netID, Name: "P1", PublicKey: "pk1",
		PrivateKey: "priv1", AllowedIPs: "10.0.0.2/32", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	now := time.Now().Truncate(time.Second)

	// Insert old snapshots (48h ago).
	for i := 0; i < 5; i++ {
		err := d.InsertSnapshot(ctx, &db.PeerSnapshot{
			PeerID:    peerID,
			Timestamp: now.Add(-48*time.Hour + time.Duration(i)*time.Minute),
			RxBytes:   int64(i * 1000),
			TxBytes:   int64(i * 500),
			Online:    true,
		})
		if err != nil {
			t.Fatalf("insert old snapshot: %v", err)
		}
	}

	// Insert recent snapshots.
	for i := 0; i < 3; i++ {
		err := d.InsertSnapshot(ctx, &db.PeerSnapshot{
			PeerID:    peerID,
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			RxBytes:   int64(i * 2000),
			TxBytes:   int64(i * 1000),
			Online:    true,
		})
		if err != nil {
			t.Fatalf("insert recent snapshot: %v", err)
		}
	}

	// Compact with 24h retention.
	compactor, err := NewCompactor(d, testLogger(), time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}

	compactor.Compact(ctx)

	// Verify old snapshots were deleted.
	remaining, err := d.ListSnapshots(ctx, peerID, now.Add(-72*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("list remaining: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected 3 remaining snapshots, got %d", len(remaining))
	}
}

func TestCompactor_Run_CancelsCleanly(t *testing.T) {
	store := &mockSnapshotStore{}
	compactor, err := NewCompactor(store, testLogger(), 50*time.Millisecond, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		compactor.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("compactor did not stop within timeout")
	}
}

// testDBForMonitor creates an in-memory SQLite DB for monitor tests.
func testDBForMonitor(t *testing.T) *db.DB {
	t.Helper()
	ctx := context.Background()
	logger := testLogger()

	d, err := db.New(ctx, ":memory:", logger, true)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(ctx, d, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
