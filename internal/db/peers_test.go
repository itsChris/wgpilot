package db

import (
	"context"
	"testing"
)

func TestPeers_CreateAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	p := testPeer(netID)
	peerID, err := d.CreatePeer(ctx, p)
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if peerID == 0 {
		t.Fatal("expected non-zero peer ID")
	}

	got, err := d.GetPeerByID(ctx, peerID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil peer")
	}
	if got.Name != "Test Peer" {
		t.Errorf("expected name %q, got %q", "Test Peer", got.Name)
	}
	if got.NetworkID != netID {
		t.Errorf("expected network_id %d, got %d", netID, got.NetworkID)
	}
	if got.AllowedIPs != "10.0.0.2/32" {
		t.Errorf("expected allowed_ips %q, got %q", "10.0.0.2/32", got.AllowedIPs)
	}
	if got.PersistentKeepalive != 25 {
		t.Errorf("expected keepalive 25, got %d", got.PersistentKeepalive)
	}
	if got.Role != "client" {
		t.Errorf("expected role %q, got %q", "client", got.Role)
	}
	if !got.Enabled {
		t.Error("expected enabled")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestPeers_GetMissing(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	got, err := d.GetPeerByID(ctx, 999)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing peer")
	}
}

func TestPeers_ListByNetworkID(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	p1 := testPeer(netID)
	p1.Name = "Peer A"
	p1.PublicKey = "key-a"
	p1.AllowedIPs = "10.0.0.2/32"

	p2 := testPeer(netID)
	p2.Name = "Peer B"
	p2.PublicKey = "key-b"
	p2.AllowedIPs = "10.0.0.3/32"

	if _, err := d.CreatePeer(ctx, p1); err != nil {
		t.Fatalf("create peer 1: %v", err)
	}
	if _, err := d.CreatePeer(ctx, p2); err != nil {
		t.Fatalf("create peer 2: %v", err)
	}

	peers, err := d.ListPeersByNetworkID(ctx, netID)
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
	if peers[0].Name != "Peer A" {
		t.Errorf("expected first peer %q, got %q", "Peer A", peers[0].Name)
	}
}

func TestPeers_Update(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	p := testPeer(netID)
	peerID, err := d.CreatePeer(ctx, p)
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	p.ID = peerID
	p.Name = "Updated Peer"
	p.Enabled = false
	if err := d.UpdatePeer(ctx, p); err != nil {
		t.Fatalf("update peer: %v", err)
	}

	got, err := d.GetPeerByID(ctx, peerID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if got.Name != "Updated Peer" {
		t.Errorf("expected name %q, got %q", "Updated Peer", got.Name)
	}
	if got.Enabled {
		t.Error("expected disabled after update")
	}
}

func TestPeers_Delete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	p := testPeer(netID)
	peerID, err := d.CreatePeer(ctx, p)
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	if err := d.DeletePeer(ctx, peerID); err != nil {
		t.Fatalf("delete peer: %v", err)
	}

	got, err := d.GetPeerByID(ctx, peerID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestPeers_ForeignKeyCascade_DeleteNetwork(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netID, err := d.CreateNetwork(ctx, testNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	p1 := testPeer(netID)
	p1.PublicKey = "key-a"
	p2 := testPeer(netID)
	p2.PublicKey = "key-b"
	p2.Name = "Peer B"
	p2.AllowedIPs = "10.0.0.3/32"

	peerID1, err := d.CreatePeer(ctx, p1)
	if err != nil {
		t.Fatalf("create peer 1: %v", err)
	}
	peerID2, err := d.CreatePeer(ctx, p2)
	if err != nil {
		t.Fatalf("create peer 2: %v", err)
	}

	// Delete the network — peers should cascade.
	if err := d.DeleteNetwork(ctx, netID); err != nil {
		t.Fatalf("delete network: %v", err)
	}

	got1, err := d.GetPeerByID(ctx, peerID1)
	if err != nil {
		t.Fatalf("get peer 1 after cascade: %v", err)
	}
	if got1 != nil {
		t.Error("expected peer 1 to be cascade-deleted")
	}

	got2, err := d.GetPeerByID(ctx, peerID2)
	if err != nil {
		t.Fatalf("get peer 2 after cascade: %v", err)
	}
	if got2 != nil {
		t.Error("expected peer 2 to be cascade-deleted")
	}
}

func TestPeers_ForeignKeyEnforced(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	p := testPeer(999) // non-existent network
	_, err := d.CreatePeer(ctx, p)
	if err == nil {
		t.Fatal("expected foreign key error for non-existent network")
	}
}
