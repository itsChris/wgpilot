package wg_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/testutil"
	"github.com/itsChris/wgpilot/internal/wg"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCreateInterface_Success(t *testing.T) {
	mockWG := &testutil.MockWireGuardController{}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	network := wg.NetworkConfig{
		ID:         1,
		Interface:  "wg0",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PrivateKey: "test-private-key",
	}

	if err := mgr.CreateInterface(context.Background(), network); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify call order: CreateWireGuardLink, AddAddress, ConfigureDevice, SetLinkUp
	linkCalls := mockLink.CallMethods()
	wgCalls := mockWG.CallMethods()

	expectedLinkCalls := []string{"CreateWireGuardLink", "AddAddress", "SetLinkUp"}
	if len(linkCalls) != len(expectedLinkCalls) {
		t.Fatalf("expected %d link calls, got %d: %v", len(expectedLinkCalls), len(linkCalls), linkCalls)
	}
	for i, expected := range expectedLinkCalls {
		if linkCalls[i] != expected {
			t.Errorf("link call %d: expected %s, got %s", i, expected, linkCalls[i])
		}
	}

	expectedWGCalls := []string{"ConfigureDevice"}
	if len(wgCalls) != len(expectedWGCalls) {
		t.Fatalf("expected %d wg calls, got %d: %v", len(expectedWGCalls), len(wgCalls), wgCalls)
	}
	if wgCalls[0] != "ConfigureDevice" {
		t.Errorf("expected ConfigureDevice, got %s", wgCalls[0])
	}

	// Verify CreateWireGuardLink was called with correct name
	if mockLink.Calls[0].Args[0] != "wg0" {
		t.Errorf("expected interface name wg0, got %v", mockLink.Calls[0].Args[0])
	}

	// Verify AddAddress was called with correct address (10.0.0.1/24)
	if mockLink.Calls[1].Args[1] != "10.0.0.1/24" {
		t.Errorf("expected address 10.0.0.1/24, got %v", mockLink.Calls[1].Args[1])
	}
}

func TestCreateInterface_LinkAddFailure(t *testing.T) {
	mockWG := &testutil.MockWireGuardController{}
	mockLink := &testutil.MockLinkManager{
		CreateWireGuardLinkFn: func(name string) error {
			return errors.New("file exists")
		},
	}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	network := wg.NetworkConfig{
		Interface:  "wg0",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PrivateKey: "key",
	}

	err = mgr.CreateInterface(context.Background(), network)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errors.New("")) {
		// Just check the error message contains the context
		if got := err.Error(); got == "" {
			t.Fatal("expected non-empty error")
		}
	}

	// Verify no further calls were made after LinkAdd failure
	if len(mockWG.CallMethods()) != 0 {
		t.Errorf("expected 0 wg calls after link failure, got %d", len(mockWG.CallMethods()))
	}
}

func TestCreateInterface_AddrAddFailure_Cleanup(t *testing.T) {
	mockWG := &testutil.MockWireGuardController{}
	mockLink := &testutil.MockLinkManager{
		AddAddressFn: func(linkName string, addr string) error {
			return errors.New("address already in use")
		},
	}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	network := wg.NetworkConfig{
		Interface:  "wg0",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PrivateKey: "key",
	}

	err = mgr.CreateInterface(context.Background(), network)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify cleanup: DeleteLink should have been called
	linkCalls := mockLink.CallMethods()
	hasDelete := false
	for _, c := range linkCalls {
		if c == "DeleteLink" {
			hasDelete = true
			break
		}
	}
	if !hasDelete {
		t.Errorf("expected DeleteLink cleanup call, got calls: %v", linkCalls)
	}
}

func TestDeleteInterface_Success(t *testing.T) {
	mockWG := &testutil.MockWireGuardController{
		DeviceFn: func(name string) (*wg.DeviceInfo, error) {
			return &wg.DeviceInfo{
				Name: name,
				Peers: []wg.WGPeerInfo{
					{PublicKey: "peer1-pubkey"},
					{PublicKey: "peer2-pubkey"},
				},
			}, nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteInterface(context.Background(), "wg0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: Device (get peers), ConfigureDevice (remove peers), SetLinkDown, DeleteLink
	wgCalls := mockWG.CallMethods()
	if len(wgCalls) != 2 {
		t.Fatalf("expected 2 wg calls, got %d: %v", len(wgCalls), wgCalls)
	}
	if wgCalls[0] != "Device" || wgCalls[1] != "ConfigureDevice" {
		t.Errorf("expected [Device, ConfigureDevice], got %v", wgCalls)
	}

	linkCalls := mockLink.CallMethods()
	if len(linkCalls) != 2 {
		t.Fatalf("expected 2 link calls, got %d: %v", len(linkCalls), linkCalls)
	}
	if linkCalls[0] != "SetLinkDown" || linkCalls[1] != "DeleteLink" {
		t.Errorf("expected [SetLinkDown, DeleteLink], got %v", linkCalls)
	}

	// Verify ConfigureDevice was called with Remove=true for both peers
	cfgCall := mockWG.Calls[1]
	cfg := cfgCall.Args[1].(wg.DeviceConfig)
	if len(cfg.Peers) != 2 {
		t.Fatalf("expected 2 peer removals, got %d", len(cfg.Peers))
	}
	for _, p := range cfg.Peers {
		if !p.Remove {
			t.Errorf("expected peer Remove=true for %s", p.PublicKey)
		}
	}
}

func TestDeleteInterface_NoPeers(t *testing.T) {
	mockWG := &testutil.MockWireGuardController{
		DeviceFn: func(name string) (*wg.DeviceInfo, error) {
			return &wg.DeviceInfo{Name: name, Peers: nil}, nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteInterface(context.Background(), "wg0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should skip ConfigureDevice (no peers to remove)
	wgCalls := mockWG.CallMethods()
	if len(wgCalls) != 1 || wgCalls[0] != "Device" {
		t.Errorf("expected only Device call, got %v", wgCalls)
	}
}

func TestAddPeer_Success(t *testing.T) {
	var capturedCfg wg.DeviceConfig
	mockWG := &testutil.MockWireGuardController{
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			capturedCfg = cfg
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	peer := wg.PeerConfig{
		Name:                "test-peer",
		PublicKey:           "test-pub-key",
		AllowedIPs:          "10.0.0.2/32",
		PersistentKeepalive: 25,
	}

	if err := mgr.AddPeer(context.Background(), "wg0", peer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedCfg.Peers) != 1 {
		t.Fatalf("expected 1 peer config, got %d", len(capturedCfg.Peers))
	}

	p := capturedCfg.Peers[0]
	if p.PublicKey != "test-pub-key" {
		t.Errorf("expected public key test-pub-key, got %s", p.PublicKey)
	}
	if !p.ReplaceAllowedIPs {
		t.Error("expected ReplaceAllowedIPs=true")
	}
	if len(p.AllowedIPs) != 1 || p.AllowedIPs[0].String() != "10.0.0.2/32" {
		t.Errorf("expected AllowedIPs [10.0.0.2/32], got %v", p.AllowedIPs)
	}
	if p.PersistentKeepaliveInterval != 25*time.Second {
		t.Errorf("expected keepalive 25s, got %v", p.PersistentKeepaliveInterval)
	}
	if p.UpdateOnly {
		t.Error("AddPeer should not set UpdateOnly")
	}
}

func TestRemovePeer_Success(t *testing.T) {
	var capturedCfg wg.DeviceConfig
	mockWG := &testutil.MockWireGuardController{
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			capturedCfg = cfg
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.RemovePeer(context.Background(), "wg0", "peer-pubkey"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedCfg.Peers) != 1 {
		t.Fatalf("expected 1 peer config, got %d", len(capturedCfg.Peers))
	}
	if !capturedCfg.Peers[0].Remove {
		t.Error("expected Remove=true")
	}
	if capturedCfg.Peers[0].PublicKey != "peer-pubkey" {
		t.Errorf("expected peer-pubkey, got %s", capturedCfg.Peers[0].PublicKey)
	}
}

func TestUpdatePeer_Success(t *testing.T) {
	var capturedCfg wg.DeviceConfig
	mockWG := &testutil.MockWireGuardController{
		ConfigureDeviceFn: func(name string, cfg wg.DeviceConfig) error {
			capturedCfg = cfg
			return nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	peer := wg.PeerConfig{
		Name:       "updated-peer",
		PublicKey:  "test-pub-key",
		AllowedIPs: "10.0.0.5/32",
	}

	if err := mgr.UpdatePeer(context.Background(), "wg0", peer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedCfg.Peers) != 1 {
		t.Fatalf("expected 1 peer config, got %d", len(capturedCfg.Peers))
	}

	p := capturedCfg.Peers[0]
	if !p.UpdateOnly {
		t.Error("UpdatePeer should set UpdateOnly=true")
	}
	if p.PublicKey != "test-pub-key" {
		t.Errorf("expected test-pub-key, got %s", p.PublicKey)
	}
}

func TestPeerStatus_Success(t *testing.T) {
	_, subnet, _ := net.ParseCIDR("10.0.0.2/32")
	recentHandshake := time.Now().Add(-1 * time.Minute)
	staleHandshake := time.Now().Add(-10 * time.Minute)

	mockWG := &testutil.MockWireGuardController{
		DeviceFn: func(name string) (*wg.DeviceInfo, error) {
			return &wg.DeviceInfo{
				Name: name,
				Peers: []wg.WGPeerInfo{
					{
						PublicKey:     "online-peer",
						Endpoint:      "1.2.3.4:51820",
						AllowedIPs:    []net.IPNet{*subnet},
						LastHandshake: recentHandshake,
						ReceiveBytes:  1024,
						TransmitBytes: 2048,
					},
					{
						PublicKey:     "offline-peer",
						LastHandshake: staleHandshake,
					},
				},
			}, nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	mgr, err := wg.NewManager(mockWG, mockLink, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	statuses, err := mgr.PeerStatus("wg0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	// First peer should be online (recent handshake)
	if !statuses[0].Online {
		t.Error("expected first peer to be online")
	}
	if statuses[0].TransferRx != 1024 {
		t.Errorf("expected 1024 rx, got %d", statuses[0].TransferRx)
	}
	if statuses[0].Endpoint != "1.2.3.4:51820" {
		t.Errorf("expected endpoint 1.2.3.4:51820, got %s", statuses[0].Endpoint)
	}

	// Second peer should be offline (stale handshake)
	if statuses[1].Online {
		t.Error("expected second peer to be offline")
	}
}

func TestNewManager_NilDependencies(t *testing.T) {
	tests := []struct {
		name   string
		wg     wg.WireGuardController
		link   wg.LinkManager
		logger *slog.Logger
	}{
		{"nil wg", nil, &testutil.MockLinkManager{}, testLogger()},
		{"nil link", &testutil.MockWireGuardController{}, nil, testLogger()},
		{"nil logger", &testutil.MockWireGuardController{}, &testutil.MockLinkManager{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := wg.NewManager(tt.wg, tt.link, tt.logger)
			if err == nil {
				t.Error("expected error for nil dependency")
			}
		})
	}
}
