package wg_test

import (
	"context"
	"net"
	"testing"

	"github.com/itsChris/wgpilot/internal/testutil"
	"github.com/itsChris/wgpilot/internal/wg"
)

func TestReconcile_MissingInterface(t *testing.T) {
	// DB has a network, but kernel has no matching WG device
	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return []wg.NetworkConfig{
				{
					ID:         1,
					Name:       "test-network",
					Interface:  "wg0",
					Subnet:     "10.0.0.0/24",
					ListenPort: 51820,
					PrivateKey: "test-key",
					Enabled:    true,
				},
			}, nil
		},
		ListPeersByNetworkIDFn: func(ctx context.Context, networkID int64) ([]wg.PeerConfig, error) {
			return nil, nil
		},
	}

	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return nil, nil // No devices in kernel
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify interface was created: CreateWireGuardLink, AddAddress, SetLinkUp
	linkCalls := mockLink.CallMethods()
	found := false
	for _, c := range linkCalls {
		if c == "CreateWireGuardLink" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CreateWireGuardLink call for missing interface, got %v", linkCalls)
	}
}

func TestReconcile_MissingPeer(t *testing.T) {
	// DB has a peer, kernel device exists but has no peers
	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return []wg.NetworkConfig{
				{
					ID:         1,
					Interface:  "wg0",
					Subnet:     "10.0.0.0/24",
					ListenPort: 51820,
					Enabled:    true,
				},
			}, nil
		},
		ListPeersByNetworkIDFn: func(ctx context.Context, networkID int64) ([]wg.PeerConfig, error) {
			return []wg.PeerConfig{
				{
					ID:         1,
					Name:       "peer1",
					PublicKey:  "peer1-pubkey",
					AllowedIPs: "10.0.0.2/32",
					Enabled:    true,
				},
			}, nil
		},
	}

	var configuredDevices []wg.DeviceConfig
	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return []*wg.DeviceInfo{
				{Name: "wg0", Peers: nil}, // Interface exists but no peers
			}, nil
		},
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			configuredDevices = append(configuredDevices, cfg)
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ConfigureDevice was called to add the missing peer
	if len(configuredDevices) == 0 {
		t.Fatal("expected ConfigureDevice call to add missing peer")
	}

	found := false
	for _, cfg := range configuredDevices {
		for _, p := range cfg.Peers {
			if p.PublicKey == "peer1-pubkey" && !p.Remove {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected peer1-pubkey to be added to kernel")
	}
}

func TestReconcile_ConfigMismatch(t *testing.T) {
	// DB peer has different AllowedIPs than kernel
	_, subnet32, _ := net.ParseCIDR("10.0.0.2/32")

	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return []wg.NetworkConfig{
				{
					ID:         1,
					Interface:  "wg0",
					Subnet:     "10.0.0.0/24",
					ListenPort: 51820,
					Enabled:    true,
				},
			}, nil
		},
		ListPeersByNetworkIDFn: func(ctx context.Context, networkID int64) ([]wg.PeerConfig, error) {
			return []wg.PeerConfig{
				{
					ID:         1,
					Name:       "peer1",
					PublicKey:  "peer1-pubkey",
					AllowedIPs: "10.0.0.5/32", // Different from kernel
					Enabled:    true,
				},
			}, nil
		},
	}

	var configuredDevices []wg.DeviceConfig
	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return []*wg.DeviceInfo{
				{
					Name: "wg0",
					Peers: []wg.WGPeerInfo{
						{
							PublicKey:  "peer1-pubkey",
							AllowedIPs: []net.IPNet{*subnet32}, // 10.0.0.2/32
						},
					},
				},
			}, nil
		},
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			configuredDevices = append(configuredDevices, cfg)
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ConfigureDevice was called with UpdateOnly to fix AllowedIPs
	found := false
	for _, cfg := range configuredDevices {
		for _, p := range cfg.Peers {
			if p.PublicKey == "peer1-pubkey" && p.UpdateOnly && p.ReplaceAllowedIPs {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected peer1 to be updated with correct AllowedIPs")
	}
}

func TestReconcile_OrphanedInterface(t *testing.T) {
	// Kernel has an interface that's not in DB — should warn but not delete
	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return nil, nil // No networks in DB
		},
	}

	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return []*wg.DeviceInfo{
				{Name: "wg-orphan", Peers: []wg.WGPeerInfo{{PublicKey: "orphan-peer"}}},
			}, nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT delete the orphaned interface (it's not managed by us)
	linkCalls := mockLink.CallMethods()
	for _, c := range linkCalls {
		if c == "DeleteLink" {
			t.Error("should not delete orphaned interface")
		}
	}
}

func TestReconcile_DisabledNetwork(t *testing.T) {
	// DB has a disabled network, but kernel has the interface — should tear down
	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return []wg.NetworkConfig{
				{
					ID:        1,
					Interface: "wg0",
					Subnet:    "10.0.0.0/24",
					Enabled:   false, // disabled
				},
			}, nil
		},
	}

	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return []*wg.DeviceInfo{
				{Name: "wg0", Peers: nil},
			}, nil
		},
		DeviceFn: func(name string) (*wg.DeviceInfo, error) {
			return &wg.DeviceInfo{Name: name, Peers: nil}, nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should tear down: SetLinkDown + DeleteLink
	linkCalls := mockLink.CallMethods()
	hasDown := false
	hasDelete := false
	for _, c := range linkCalls {
		if c == "SetLinkDown" {
			hasDown = true
		}
		if c == "DeleteLink" {
			hasDelete = true
		}
	}
	if !hasDown || !hasDelete {
		t.Errorf("expected teardown (SetLinkDown + DeleteLink) for disabled network, got %v", linkCalls)
	}
}

func TestReconcile_OrphanedPeer(t *testing.T) {
	// Kernel has a peer that's not in DB — should remove it
	_, peerSubnet, _ := net.ParseCIDR("10.0.0.2/32")

	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return []wg.NetworkConfig{
				{
					ID:         1,
					Interface:  "wg0",
					Subnet:     "10.0.0.0/24",
					ListenPort: 51820,
					Enabled:    true,
				},
			}, nil
		},
		ListPeersByNetworkIDFn: func(ctx context.Context, networkID int64) ([]wg.PeerConfig, error) {
			return nil, nil // No peers in DB
		},
	}

	var removedPeers []string
	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return []*wg.DeviceInfo{
				{
					Name: "wg0",
					Peers: []wg.WGPeerInfo{
						{PublicKey: "orphan-peer-key", AllowedIPs: []net.IPNet{*peerSubnet}},
					},
				},
			}, nil
		},
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			for _, p := range cfg.Peers {
				if p.Remove {
					removedPeers = append(removedPeers, p.PublicKey)
				}
			}
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(removedPeers) != 1 || removedPeers[0] != "orphan-peer-key" {
		t.Errorf("expected orphan-peer-key to be removed, got %v", removedPeers)
	}
}

func TestReconcile_MatchingState(t *testing.T) {
	// Everything matches — no corrective actions
	_, peerSubnet, _ := net.ParseCIDR("10.0.0.2/32")

	store := &testutil.MockNetworkStore{
		ListNetworksFn: func(ctx context.Context) ([]wg.NetworkConfig, error) {
			return []wg.NetworkConfig{
				{
					ID:         1,
					Interface:  "wg0",
					Subnet:     "10.0.0.0/24",
					ListenPort: 51820,
					Enabled:    true,
				},
			}, nil
		},
		ListPeersByNetworkIDFn: func(ctx context.Context, networkID int64) ([]wg.PeerConfig, error) {
			return []wg.PeerConfig{
				{
					ID:         1,
					Name:       "peer1",
					PublicKey:  "peer1-pubkey",
					AllowedIPs: "10.0.0.2/32",
					Enabled:    true,
				},
			}, nil
		},
	}

	configCallCount := 0
	mockWG := &testutil.MockWireGuardController{
		DevicesFn: func() ([]*wg.DeviceInfo, error) {
			return []*wg.DeviceInfo{
				{
					Name: "wg0",
					Peers: []wg.WGPeerInfo{
						{
							PublicKey:  "peer1-pubkey",
							AllowedIPs: []net.IPNet{*peerSubnet},
						},
					},
				},
			}, nil
		},
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			configCallCount++
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Reconcile(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No ConfigureDevice calls should be made (everything matches)
	if configCallCount != 0 {
		t.Errorf("expected 0 ConfigureDevice calls when state matches, got %d", configCallCount)
	}

	// No link operations should be made
	if len(mockLink.CallMethods()) != 0 {
		t.Errorf("expected 0 link calls when state matches, got %v", mockLink.CallMethods())
	}
}
