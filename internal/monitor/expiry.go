package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
)

// ExpiryStore abstracts database operations needed by the expiry checker.
type ExpiryStore interface {
	ListExpiredPeers(ctx context.Context) ([]db.Peer, error)
	UpdatePeer(ctx context.Context, p *db.Peer) error
	GetNetworkByID(ctx context.Context, id int64) (*db.Network, error)
}

// PeerRemover abstracts WireGuard peer removal for the expiry checker.
type PeerRemover interface {
	RemovePeer(ctx context.Context, iface, publicKey string) error
}

// ExpiryChecker periodically checks for expired peers and disables them.
type ExpiryChecker struct {
	store    ExpiryStore
	remover  PeerRemover
	logger   *slog.Logger
	interval time.Duration
}

// NewExpiryChecker creates an ExpiryChecker that runs at the given interval.
func NewExpiryChecker(store ExpiryStore, remover PeerRemover, logger *slog.Logger, interval time.Duration) (*ExpiryChecker, error) {
	if store == nil {
		return nil, fmt.Errorf("new expiry checker: store is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("new expiry checker: logger is required")
	}
	return &ExpiryChecker{
		store:    store,
		remover:  remover,
		logger:   logger.With("component", "expiry"),
		interval: interval,
	}, nil
}

// Run starts the expiry check loop. It blocks until ctx is cancelled.
func (e *ExpiryChecker) Run(ctx context.Context) {
	taskID := logging.GenerateTaskID("expiry")
	ctx = logging.WithTaskID(ctx, taskID)

	e.logger.Info("expiry_checker_started",
		"interval", e.interval.String(),
		"task_id", taskID,
	)

	e.check(ctx)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("expiry_checker_stopped", "task_id", taskID)
			return
		case <-ticker.C:
			e.check(ctx)
		}
	}
}

func (e *ExpiryChecker) check(ctx context.Context) {
	expired, err := e.store.ListExpiredPeers(ctx)
	if err != nil {
		e.logger.Error("expiry_list_failed",
			"error", err,
			"operation", "check",
		)
		return
	}

	for _, peer := range expired {
		peer.Enabled = false
		if err := e.store.UpdatePeer(ctx, &peer); err != nil {
			e.logger.Error("expiry_disable_failed",
				"error", err,
				"peer_id", peer.ID,
				"peer_name", peer.Name,
				"operation", "check",
			)
			continue
		}

		// Remove from WireGuard.
		if e.remover != nil {
			network, err := e.store.GetNetworkByID(ctx, peer.NetworkID)
			if err == nil && network != nil && network.Enabled {
				if rmErr := e.remover.RemovePeer(ctx, network.Interface, peer.PublicKey); rmErr != nil {
					e.logger.Error("expiry_remove_peer_failed",
						"error", rmErr,
						"peer_id", peer.ID,
						"interface", network.Interface,
						"operation", "check",
					)
				}
			}
		}

		e.logger.Info("peer_expired_disabled",
			"peer_id", peer.ID,
			"peer_name", peer.Name,
			"network_id", peer.NetworkID,
			"expires_at", peer.ExpiresAt,
			"operation", "check",
		)
	}

	if len(expired) > 0 {
		e.logger.Info("expiry_check_complete",
			"disabled_count", len(expired),
			"operation", "check",
		)
	}
}
