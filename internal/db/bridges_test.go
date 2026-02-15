package db

import (
	"context"
	"testing"
)

func TestBridges_CreateAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	// Create two networks first.
	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)

	b := &Bridge{
		NetworkAID:   netAID,
		NetworkBID:   netBID,
		Direction:    "bidirectional",
		AllowedCIDRs: "",
		Enabled:      true,
	}
	id, err := d.CreateBridge(ctx, b)
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := d.GetBridgeByID(ctx, id)
	if err != nil {
		t.Fatalf("get bridge: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil bridge")
	}
	if got.NetworkAID != netAID {
		t.Errorf("expected network_a_id=%d, got %d", netAID, got.NetworkAID)
	}
	if got.NetworkBID != netBID {
		t.Errorf("expected network_b_id=%d, got %d", netBID, got.NetworkBID)
	}
	if got.Direction != "bidirectional" {
		t.Errorf("expected direction=bidirectional, got %q", got.Direction)
	}
	if !got.Enabled {
		t.Error("expected enabled=true")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestBridges_GetMissing(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	got, err := d.GetBridgeByID(ctx, 999)
	if err != nil {
		t.Fatalf("get bridge: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing bridge")
	}
}

func TestBridges_List(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)
	netCID := createTestNetwork(t, d, ctx, "wg2", 51822)

	if _, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge 1: %v", err)
	}
	if _, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netBID, NetworkBID: netCID, Direction: "a_to_b", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge 2: %v", err)
	}

	bridges, err := d.ListBridges(ctx)
	if err != nil {
		t.Fatalf("list bridges: %v", err)
	}
	if len(bridges) != 2 {
		t.Fatalf("expected 2 bridges, got %d", len(bridges))
	}
}

func TestBridges_ListByNetworkID(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)
	netCID := createTestNetwork(t, d, ctx, "wg2", 51822)

	if _, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge 1: %v", err)
	}
	if _, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netBID, NetworkBID: netCID, Direction: "a_to_b", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge 2: %v", err)
	}

	// Network B should appear in both bridges.
	bridges, err := d.ListBridgesByNetworkID(ctx, netBID)
	if err != nil {
		t.Fatalf("list bridges by network: %v", err)
	}
	if len(bridges) != 2 {
		t.Fatalf("expected 2 bridges for network B, got %d", len(bridges))
	}

	// Network A should appear in 1 bridge.
	bridges, err = d.ListBridgesByNetworkID(ctx, netAID)
	if err != nil {
		t.Fatalf("list bridges by network: %v", err)
	}
	if len(bridges) != 1 {
		t.Fatalf("expected 1 bridge for network A, got %d", len(bridges))
	}
}

func TestBridges_ExistsBetween(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)

	// Should not exist before creation.
	exists, err := d.BridgeExistsBetween(ctx, netAID, netBID)
	if err != nil {
		t.Fatalf("check exists: %v", err)
	}
	if exists {
		t.Error("expected no bridge before creation")
	}

	if _, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	// Should exist in both orders.
	exists, err = d.BridgeExistsBetween(ctx, netAID, netBID)
	if err != nil {
		t.Fatalf("check exists: %v", err)
	}
	if !exists {
		t.Error("expected bridge to exist (a, b)")
	}

	exists, err = d.BridgeExistsBetween(ctx, netBID, netAID)
	if err != nil {
		t.Fatalf("check exists reverse: %v", err)
	}
	if !exists {
		t.Error("expected bridge to exist (b, a)")
	}
}

func TestBridges_Delete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)

	id, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	if err := d.DeleteBridge(ctx, id); err != nil {
		t.Fatalf("delete bridge: %v", err)
	}

	got, err := d.GetBridgeByID(ctx, id)
	if err != nil {
		t.Fatalf("get bridge after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestBridges_DuplicateReturnsError(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)

	if _, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	// Duplicate with same order should fail (UNIQUE constraint).
	_, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "a_to_b", Enabled: true,
	})
	if err == nil {
		t.Fatal("expected error for duplicate bridge")
	}
}

func TestBridges_CascadeOnNetworkDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	netAID := createTestNetwork(t, d, ctx, "wg0", 51820)
	netBID := createTestNetwork(t, d, ctx, "wg1", 51821)

	id, err := d.CreateBridge(ctx, &Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	// Delete network A — bridge should be cascade-deleted.
	if err := d.DeleteNetwork(ctx, netAID); err != nil {
		t.Fatalf("delete network: %v", err)
	}

	got, err := d.GetBridgeByID(ctx, id)
	if err != nil {
		t.Fatalf("get bridge after cascade: %v", err)
	}
	if got != nil {
		t.Fatal("expected bridge to be cascade-deleted")
	}
}

// createTestNetwork is a helper that creates a network with unique interface and port.
func createTestNetwork(t *testing.T, d *DB, ctx context.Context, iface string, port int) int64 {
	t.Helper()
	id, err := d.CreateNetwork(ctx, &Network{
		Name:       "Network " + iface,
		Interface:  iface,
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: port,
		PrivateKey: "priv-" + iface,
		PublicKey:  "pub-" + iface,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create test network %s: %v", iface, err)
	}
	return id
}
