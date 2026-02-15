package db

import (
	"context"
	"testing"
)

func TestNetworks_CreateAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n := testNetwork()
	id, err := d.CreateNetwork(ctx, n)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := d.GetNetworkByID(ctx, id)
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil network")
	}
	if got.Name != "Test Network" {
		t.Errorf("expected name %q, got %q", "Test Network", got.Name)
	}
	if got.Interface != "wg0" {
		t.Errorf("expected interface %q, got %q", "wg0", got.Interface)
	}
	if got.Mode != "gateway" {
		t.Errorf("expected mode %q, got %q", "gateway", got.Mode)
	}
	if got.Subnet != "10.0.0.0/24" {
		t.Errorf("expected subnet %q, got %q", "10.0.0.0/24", got.Subnet)
	}
	if got.ListenPort != 51820 {
		t.Errorf("expected port %d, got %d", 51820, got.ListenPort)
	}
	if !got.NATEnabled {
		t.Error("expected NAT enabled")
	}
	if got.InterPeerRouting {
		t.Error("expected inter-peer routing disabled")
	}
	if !got.Enabled {
		t.Error("expected enabled")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestNetworks_GetMissing(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	got, err := d.GetNetworkByID(ctx, 999)
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing network")
	}
}

func TestNetworks_List(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n1 := testNetwork()
	n2 := testNetwork()
	n2.Interface = "wg1"
	n2.ListenPort = 51821
	n2.Name = "Second Network"

	if _, err := d.CreateNetwork(ctx, n1); err != nil {
		t.Fatalf("create n1: %v", err)
	}
	if _, err := d.CreateNetwork(ctx, n2); err != nil {
		t.Fatalf("create n2: %v", err)
	}

	networks, err := d.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("list networks: %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(networks))
	}
	if networks[0].Name != "Test Network" {
		t.Errorf("expected first network %q, got %q", "Test Network", networks[0].Name)
	}
	if networks[1].Name != "Second Network" {
		t.Errorf("expected second network %q, got %q", "Second Network", networks[1].Name)
	}
}

func TestNetworks_Update(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n := testNetwork()
	id, err := d.CreateNetwork(ctx, n)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	n.ID = id
	n.Name = "Updated Name"
	n.NATEnabled = false
	if err := d.UpdateNetwork(ctx, n); err != nil {
		t.Fatalf("update network: %v", err)
	}

	got, err := d.GetNetworkByID(ctx, id)
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("expected name %q, got %q", "Updated Name", got.Name)
	}
	if got.NATEnabled {
		t.Error("expected NAT disabled after update")
	}
}

func TestNetworks_Delete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n := testNetwork()
	id, err := d.CreateNetwork(ctx, n)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	if err := d.DeleteNetwork(ctx, id); err != nil {
		t.Fatalf("delete network: %v", err)
	}

	got, err := d.GetNetworkByID(ctx, id)
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestNetworks_UniqueInterface(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n1 := testNetwork()
	n2 := testNetwork()
	n2.ListenPort = 51821

	if _, err := d.CreateNetwork(ctx, n1); err != nil {
		t.Fatalf("create n1: %v", err)
	}

	_, err := d.CreateNetwork(ctx, n2)
	if err == nil {
		t.Fatal("expected error for duplicate interface name")
	}
}

func TestNetworks_UniqueListenPort(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n1 := testNetwork()
	n2 := testNetwork()
	n2.Interface = "wg1"

	if _, err := d.CreateNetwork(ctx, n1); err != nil {
		t.Fatalf("create n1: %v", err)
	}

	_, err := d.CreateNetwork(ctx, n2)
	if err == nil {
		t.Fatal("expected error for duplicate listen port")
	}
}
