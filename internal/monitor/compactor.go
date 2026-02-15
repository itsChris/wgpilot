package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/itsChris/wgpilot/internal/logging"
)

// Compactor periodically deletes old peer snapshots.
type Compactor struct {
	store     SnapshotStore
	logger    *slog.Logger
	interval  time.Duration
	retention time.Duration
}

// NewCompactor creates a Compactor that runs at the given interval
// and deletes snapshots older than retention.
func NewCompactor(store SnapshotStore, logger *slog.Logger, interval, retention time.Duration) (*Compactor, error) {
	if store == nil {
		return nil, fmt.Errorf("new compactor: store is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("new compactor: logger is required")
	}
	return &Compactor{
		store:     store,
		logger:    logger.With("component", "compactor"),
		interval:  interval,
		retention: retention,
	}, nil
}

// Run starts the compaction loop. It blocks until ctx is cancelled.
func (c *Compactor) Run(ctx context.Context) {
	taskID := logging.GenerateTaskID("compactor")
	ctx = logging.WithTaskID(ctx, taskID)

	c.logger.Info("compactor_started",
		"interval", c.interval.String(),
		"retention", c.retention.String(),
		"task_id", taskID,
	)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("compactor_stopped", "task_id", taskID)
			return
		case <-ticker.C:
			c.compact(ctx)
		}
	}
}

// Compact executes a single compaction cycle. Exported for testing.
func (c *Compactor) Compact(ctx context.Context) {
	c.compact(ctx)
}

func (c *Compactor) compact(ctx context.Context) {
	cutoff := time.Now().Add(-c.retention)

	deleted, err := c.store.CompactSnapshots(ctx, cutoff)
	if err != nil {
		c.logger.Error("compaction_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "compact",
			"cutoff", cutoff.Unix(),
		)
		return
	}

	if deleted > 0 {
		c.logger.Info("compaction_complete",
			"deleted", deleted,
			"cutoff", cutoff.Unix(),
			"operation", "compact",
		)
	} else {
		c.logger.Debug("compaction_noop",
			"cutoff", cutoff.Unix(),
			"operation", "compact",
		)
	}
}
