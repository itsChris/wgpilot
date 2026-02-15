package db

import (
	"context"
	"fmt"
	"time"
)

// PeerSnapshot represents a row in the peer_snapshots table.
type PeerSnapshot struct {
	PeerID    int64
	Timestamp time.Time
	RxBytes   int64
	TxBytes   int64
	Online    bool
}

// InsertSnapshot inserts a peer snapshot record.
func (d *DB) InsertSnapshot(ctx context.Context, s *PeerSnapshot) error {
	_, err := d.ExecContext(ctx,
		"INSERT INTO peer_snapshots (peer_id, timestamp, rx_bytes, tx_bytes, online) VALUES (?, ?, ?, ?, ?)",
		s.PeerID, s.Timestamp.Unix(), s.RxBytes, s.TxBytes, s.Online,
	)
	if err != nil {
		return fmt.Errorf("db: insert snapshot for peer %d: %w", s.PeerID, err)
	}
	return nil
}

// ListSnapshots returns snapshots for a peer within a time range, ordered by timestamp.
func (d *DB) ListSnapshots(ctx context.Context, peerID int64, from, to time.Time) ([]PeerSnapshot, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT peer_id, timestamp, rx_bytes, tx_bytes, online
		FROM peer_snapshots
		WHERE peer_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp`,
		peerID, from.Unix(), to.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("db: list snapshots for peer %d: %w", peerID, err)
	}
	defer rows.Close()

	var snapshots []PeerSnapshot
	for rows.Next() {
		var s PeerSnapshot
		var ts int64
		if err := rows.Scan(&s.PeerID, &ts, &s.RxBytes, &s.TxBytes, &s.Online); err != nil {
			return nil, fmt.Errorf("db: scan snapshot: %w", err)
		}
		s.Timestamp = time.Unix(ts, 0)
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// CompactSnapshots deletes snapshots older than the given cutoff time.
// Returns the number of rows deleted.
func (d *DB) CompactSnapshots(ctx context.Context, before time.Time) (int64, error) {
	result, err := d.ExecContext(ctx,
		"DELETE FROM peer_snapshots WHERE timestamp < ?",
		before.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("db: compact snapshots: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("db: compact snapshots rows affected: %w", err)
	}
	return n, nil
}
