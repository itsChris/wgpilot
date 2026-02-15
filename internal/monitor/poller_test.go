package monitor

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/wg"
)

type mockSnapshotStore struct {
	mu        sync.Mutex
	networks  []db.Network
	peers     map[int64][]db.Peer // networkID -> peers
	snapshots []*db.PeerSnapshot
	compacted int64
}

func (m *mockSnapshotStore) ListNetworks(ctx context.Context) ([]db.Network, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.networks, nil
}

func (m *mockSnapshotStore) ListPeersByNetworkID(ctx context.Context, networkID int64) ([]db.Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peers[networkID], nil
}

func (m *mockSnapshotStore) InsertSnapshot(ctx context.Context, s *db.PeerSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots = append(m.snapshots, s)
	return nil
}

func (m *mockSnapshotStore) CompactSnapshots(ctx context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compacted++
	return m.compacted, nil
}

func (m *mockSnapshotStore) snapshotCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.snapshots)
}

type mockStatusProvider struct {
	mu       sync.Mutex
	statuses map[string][]wg.PeerStatus
}

func (m *mockStatusProvider) PeerStatus(iface string) ([]wg.PeerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statuses[iface], nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewPoller_NilStore(t *testing.T) {
	_, err := NewPoller(nil, &mockStatusProvider{}, testLogger(), time.Second)
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestNewPoller_NilStatus(t *testing.T) {
	_, err := NewPoller(&mockSnapshotStore{}, nil, testLogger(), time.Second)
	if err == nil {
		t.Fatal("expected error for nil status provider")
	}
}

func TestNewPoller_NilLogger(t *testing.T) {
	_, err := NewPoller(&mockSnapshotStore{}, &mockStatusProvider{}, nil, time.Second)
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

func TestPoller_Poll_StoresSnapshots(t *testing.T) {
	store := &mockSnapshotStore{
		networks: []db.Network{
			{ID: 1, Name: "Test", Interface: "wg0", Enabled: true},
		},
		peers: map[int64][]db.Peer{
			1: {
				{ID: 10, NetworkID: 1, PublicKey: "pubkey1", Name: "Peer1"},
				{ID: 11, NetworkID: 1, PublicKey: "pubkey2", Name: "Peer2"},
			},
		},
	}

	status := &mockStatusProvider{
		statuses: map[string][]wg.PeerStatus{
			"wg0": {
				{PublicKey: "pubkey1", Online: true, TransferRx: 1000, TransferTx: 2000, LastHandshake: time.Now()},
				{PublicKey: "pubkey2", Online: false, TransferRx: 500, TransferTx: 100},
			},
		},
	}

	poller, err := NewPoller(store, status, testLogger(), time.Second)
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	poller.Poll(context.Background())

	if got := store.snapshotCount(); got != 2 {
		t.Fatalf("expected 2 snapshots, got %d", got)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	// Verify first snapshot.
	s := store.snapshots[0]
	if s.PeerID != 10 {
		t.Errorf("expected peer_id=10, got %d", s.PeerID)
	}
	if s.RxBytes != 1000 {
		t.Errorf("expected rx=1000, got %d", s.RxBytes)
	}
	if !s.Online {
		t.Error("expected online=true for first peer")
	}

	// Verify second snapshot.
	s = store.snapshots[1]
	if s.PeerID != 11 {
		t.Errorf("expected peer_id=11, got %d", s.PeerID)
	}
	if s.Online {
		t.Error("expected online=false for second peer")
	}
}

func TestPoller_Poll_DetectsTransitions(t *testing.T) {
	store := &mockSnapshotStore{
		networks: []db.Network{
			{ID: 1, Name: "Test", Interface: "wg0", Enabled: true},
		},
		peers: map[int64][]db.Peer{
			1: {
				{ID: 10, NetworkID: 1, PublicKey: "pubkey1", Name: "Peer1"},
			},
		},
	}

	status := &mockStatusProvider{
		statuses: map[string][]wg.PeerStatus{
			"wg0": {
				{PublicKey: "pubkey1", Online: true, TransferRx: 1000, TransferTx: 2000},
			},
		},
	}

	poller, err := NewPoller(store, status, testLogger(), time.Second)
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	// First poll: establishes baseline.
	poller.Poll(context.Background())

	poller.mu.Lock()
	if !poller.prevState[10] {
		t.Error("expected prevState[10]=true after first poll")
	}
	poller.mu.Unlock()

	// Change peer to offline.
	status.mu.Lock()
	status.statuses["wg0"][0].Online = false
	status.mu.Unlock()

	// Second poll: should detect transition.
	poller.Poll(context.Background())

	poller.mu.Lock()
	if poller.prevState[10] {
		t.Error("expected prevState[10]=false after second poll")
	}
	poller.mu.Unlock()
}

func TestPoller_Poll_SkipsDisabledNetworks(t *testing.T) {
	store := &mockSnapshotStore{
		networks: []db.Network{
			{ID: 1, Name: "Disabled", Interface: "wg0", Enabled: false},
		},
	}

	status := &mockStatusProvider{
		statuses: map[string][]wg.PeerStatus{
			"wg0": {{PublicKey: "pubkey1", Online: true}},
		},
	}

	poller, err := NewPoller(store, status, testLogger(), time.Second)
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	poller.Poll(context.Background())

	if got := store.snapshotCount(); got != 0 {
		t.Fatalf("expected 0 snapshots for disabled network, got %d", got)
	}
}

func TestPoller_Poll_SkipsUnknownPeers(t *testing.T) {
	store := &mockSnapshotStore{
		networks: []db.Network{
			{ID: 1, Name: "Test", Interface: "wg0", Enabled: true},
		},
		peers: map[int64][]db.Peer{
			1: {
				{ID: 10, NetworkID: 1, PublicKey: "known-key", Name: "Known"},
			},
		},
	}

	status := &mockStatusProvider{
		statuses: map[string][]wg.PeerStatus{
			"wg0": {
				{PublicKey: "known-key", Online: true, TransferRx: 100},
				{PublicKey: "unknown-key", Online: true, TransferRx: 200},
			},
		},
	}

	poller, err := NewPoller(store, status, testLogger(), time.Second)
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	poller.Poll(context.Background())

	if got := store.snapshotCount(); got != 1 {
		t.Fatalf("expected 1 snapshot (unknown peer skipped), got %d", got)
	}
}

func TestPoller_Run_CancelsCleanly(t *testing.T) {
	store := &mockSnapshotStore{
		networks: []db.Network{},
	}
	status := &mockStatusProvider{}

	poller, err := NewPoller(store, status, testLogger(), 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		poller.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Poller stopped cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not stop within timeout")
	}
}
