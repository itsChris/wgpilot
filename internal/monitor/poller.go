package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
	"github.com/itsChris/wgpilot/internal/wg"
)

// StatusProvider abstracts peer status retrieval for testability.
type StatusProvider interface {
	PeerStatus(iface string) ([]wg.PeerStatus, error)
}

// SnapshotStore abstracts database operations needed by the monitor.
type SnapshotStore interface {
	ListNetworks(ctx context.Context) ([]db.Network, error)
	ListPeersByNetworkID(ctx context.Context, networkID int64) ([]db.Peer, error)
	InsertSnapshot(ctx context.Context, s *db.PeerSnapshot) error
	CompactSnapshots(ctx context.Context, before time.Time) (int64, error)
}

// PeerEvent represents a peer status update for SSE subscribers.
type PeerEvent struct {
	PeerID        int64  `json:"peer_id"`
	Name          string `json:"name"`
	Online        bool   `json:"online"`
	LastHandshake int64  `json:"last_handshake"`
	TransferRx    int64  `json:"transfer_rx"`
	TransferTx    int64  `json:"transfer_tx"`
}

// Poller periodically polls WireGuard peer status, stores snapshots,
// and detects online/offline transitions.
type Poller struct {
	store    SnapshotStore
	status   StatusProvider
	logger   *slog.Logger
	interval time.Duration

	mu        sync.Mutex
	prevState map[int64]bool // peer ID -> online
}

// NewPoller creates a Poller that polls at the given interval.
func NewPoller(store SnapshotStore, status StatusProvider, logger *slog.Logger, interval time.Duration) (*Poller, error) {
	if store == nil {
		return nil, fmt.Errorf("new poller: store is required")
	}
	if status == nil {
		return nil, fmt.Errorf("new poller: status provider is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("new poller: logger is required")
	}
	return &Poller{
		store:     store,
		status:    status,
		logger:    logger.With("component", "monitor"),
		interval:  interval,
		prevState: make(map[int64]bool),
	}, nil
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	taskID := logging.GenerateTaskID("poller")
	ctx = logging.WithTaskID(ctx, taskID)

	p.logger.Info("poller_started",
		"interval", p.interval.String(),
		"task_id", taskID,
	)

	p.poll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("poller_stopped", "task_id", taskID)
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// Poll executes a single poll cycle. Exported for testing.
func (p *Poller) Poll(ctx context.Context) {
	p.poll(ctx)
}

func (p *Poller) poll(ctx context.Context) {
	networks, err := p.store.ListNetworks(ctx)
	if err != nil {
		p.logger.Error("poll_list_networks_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "poll",
		)
		return
	}

	now := time.Now()

	for _, net := range networks {
		if !net.Enabled {
			continue
		}

		statuses, err := p.status.PeerStatus(net.Interface)
		if err != nil {
			p.logger.Error("poll_peer_status_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "poll",
				"network_id", net.ID,
				"interface", net.Interface,
			)
			continue
		}

		peers, err := p.store.ListPeersByNetworkID(ctx, net.ID)
		if err != nil {
			p.logger.Error("poll_list_peers_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "poll",
				"network_id", net.ID,
			)
			continue
		}

		peerByKey := make(map[string]db.Peer, len(peers))
		for _, peer := range peers {
			peerByKey[peer.PublicKey] = peer
		}

		for _, s := range statuses {
			peer, ok := peerByKey[s.PublicKey]
			if !ok {
				continue
			}

			snapshot := &db.PeerSnapshot{
				PeerID:    peer.ID,
				Timestamp: now,
				RxBytes:   s.TransferRx,
				TxBytes:   s.TransferTx,
				Online:    s.Online,
			}
			if err := p.store.InsertSnapshot(ctx, snapshot); err != nil {
				p.logger.Error("poll_insert_snapshot_failed",
					"error", err,
					"error_type", fmt.Sprintf("%T", err),
					"operation", "poll",
					"peer_id", peer.ID,
				)
				continue
			}

			p.mu.Lock()
			prev, known := p.prevState[peer.ID]
			if known && prev != s.Online {
				if s.Online {
					p.logger.Info("peer_online",
						"peer_id", peer.ID,
						"peer_name", peer.Name,
						"network", net.Interface,
						"operation", "poll",
					)
				} else {
					p.logger.Info("peer_offline",
						"peer_id", peer.ID,
						"peer_name", peer.Name,
						"network", net.Interface,
						"operation", "poll",
					)
				}
			}
			p.prevState[peer.ID] = s.Online
			p.mu.Unlock()
		}
	}
}
