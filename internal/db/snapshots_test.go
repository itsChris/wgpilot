package db

import (
	"context"
	"testing"
	"time"
)

func TestSnapshots_InsertAndList(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	peerID, err := d.CreatePeer(ctx, testPeer(netID))
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		s := testSnapshot(peerID, now.Add(time.Duration(i)*time.Minute))
		s.RxBytes = int64(i * 1000)
		s.TxBytes = int64(i * 2000)
		if err := d.InsertSnapshot(ctx, s); err != nil {
			t.Fatalf("insert snapshot %d: %v", i, err)
		}
	}

	// Query all 5.
	snapshots, err := d.ListSnapshots(ctx, peerID, now, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snapshots) != 5 {
		t.Fatalf("expected 5 snapshots, got %d", len(snapshots))
	}

	// Verify ordering.
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i].Timestamp.Before(snapshots[i-1].Timestamp) {
			t.Error("snapshots not in chronological order")
		}
	}

	// Query a subset.
	snapshots, err = d.ListSnapshots(ctx, peerID, now.Add(2*time.Minute), now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("list snapshots subset: %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots in range, got %d", len(snapshots))
	}
}

func TestSnapshots_Compact(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	peerID, err := d.CreatePeer(ctx, testPeer(netID))
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	now := time.Now().Truncate(time.Second)

	// Insert 10 snapshots: 5 old, 5 recent.
	for i := 0; i < 5; i++ {
		s := testSnapshot(peerID, now.Add(-48*time.Hour+time.Duration(i)*time.Minute))
		if err := d.InsertSnapshot(ctx, s); err != nil {
			t.Fatalf("insert old snapshot %d: %v", i, err)
		}
	}
	for i := 0; i < 5; i++ {
		s := testSnapshot(peerID, now.Add(time.Duration(i)*time.Minute))
		if err := d.InsertSnapshot(ctx, s); err != nil {
			t.Fatalf("insert recent snapshot %d: %v", i, err)
		}
	}

	// Compact everything older than 24 hours.
	cutoff := now.Add(-24 * time.Hour)
	deleted, err := d.CompactSnapshots(ctx, cutoff)
	if err != nil {
		t.Fatalf("compact snapshots: %v", err)
	}
	if deleted != 5 {
		t.Fatalf("expected 5 deleted, got %d", deleted)
	}

	// Verify only recent snapshots remain.
	remaining, err := d.ListSnapshots(ctx, peerID, now.Add(-72*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("list remaining: %v", err)
	}
	if len(remaining) != 5 {
		t.Fatalf("expected 5 remaining, got %d", len(remaining))
	}
}

func TestSnapshots_CompactEmpty(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	deleted, err := d.CompactSnapshots(ctx, time.Now())
	if err != nil {
		t.Fatalf("compact empty: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

func TestSnapshots_CascadeOnPeerDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	peerID, err := d.CreatePeer(ctx, testPeer(netID))
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	if err := d.InsertSnapshot(ctx, testSnapshot(peerID, now)); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	// Delete the peer — snapshots should cascade.
	if err := d.DeletePeer(ctx, peerID); err != nil {
		t.Fatalf("delete peer: %v", err)
	}

	snapshots, err := d.ListSnapshots(ctx, peerID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("list snapshots after cascade: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected 0 snapshots after cascade, got %d", len(snapshots))
	}
}

func TestSnapshots_CascadeOnNetworkDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	peerID, err := d.CreatePeer(ctx, testPeer(netID))
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	if err := d.InsertSnapshot(ctx, testSnapshot(peerID, now)); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	// Delete the network — peers and snapshots should cascade.
	if err := d.DeleteNetwork(ctx, netID); err != nil {
		t.Fatalf("delete network: %v", err)
	}

	snapshots, err := d.ListSnapshots(ctx, peerID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("list snapshots after network cascade: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected 0 snapshots after network cascade, got %d", len(snapshots))
	}
}
