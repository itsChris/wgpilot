package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/itsChris/wgpilot/internal/logging"
	_ "modernc.org/sqlite"
)

const slowQueryThreshold = 100 * time.Millisecond

// DB wraps a sql.DB with logging and query helpers.
type DB struct {
	conn    *sql.DB
	logger  *slog.Logger
	devMode bool
}

// New opens a SQLite database and configures WAL mode, foreign keys,
// and busy timeout.
func New(ctx context.Context, dsn string, logger *slog.Logger, devMode bool) (*DB, error) {
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", dsn, err)
	}

	// Single writer connection for SQLite.
	conn.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := conn.ExecContext(ctx, p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("db: exec %q: %w", p, err)
		}
	}

	logger.Info("database_opened",
		"dsn", dsn,
		"component", "db",
	)

	return &DB{
		conn:    conn,
		logger:  logger,
		devMode: devMode,
	}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Conn returns the underlying *sql.DB for use by the migration runner.
func (d *DB) Conn() *sql.DB {
	return d.conn
}

// ExecContext executes a query that doesn't return rows.
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := d.conn.ExecContext(ctx, query, args...)
	d.logQuery(ctx, "exec", query, args, time.Since(start), err)
	return result, err
}

// QueryContext executes a query that returns rows.
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := d.conn.QueryContext(ctx, query, args...)
	d.logQuery(ctx, "query", query, args, time.Since(start), err)
	return rows, err
}

// QueryRowContext executes a query that returns at most one row.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *timedRow {
	start := time.Now()
	row := d.conn.QueryRowContext(ctx, query, args...)
	return &timedRow{
		row:   row,
		db:    d,
		ctx:   ctx,
		op:    "query_row",
		query: query,
		args:  args,
		start: start,
	}
}

// BeginTx starts a transaction with logging.
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	start := time.Now()
	tx, err := d.conn.BeginTx(ctx, opts)
	if err != nil {
		d.logger.Error("sql_begin_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"component", "db",
		)
		return nil, fmt.Errorf("db: begin tx: %w", err)
	}
	if d.devMode {
		d.logger.Debug("sql_begin",
			"duration_ms", time.Since(start).Milliseconds(),
			"component", "db",
		)
	}
	return &Tx{tx: tx, db: d, start: start, ctx: ctx}, nil
}

func (d *DB) logQuery(ctx context.Context, op, query string, args []any, duration time.Duration, err error) {
	requestID := logging.RequestID(ctx)

	if d.devMode {
		d.logger.Debug("sql_"+op,
			"request_id", requestID,
			"query", query,
			"args", fmt.Sprintf("%v", args),
			"duration_ms", duration.Milliseconds(),
			"error", err,
			"component", "db",
		)
	}

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		d.logger.Error("sql_"+op+"_failed",
			"request_id", requestID,
			"query", query,
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"duration_ms", duration.Milliseconds(),
			"component", "db",
		)
	}

	if duration > slowQueryThreshold {
		d.logger.Warn("slow_query",
			"request_id", requestID,
			"query", query,
			"duration_ms", duration.Milliseconds(),
			"component", "db",
		)
	}
}

// TableCounts returns row counts for the given tables. Tables that don't
// exist or can't be queried are silently omitted from the result.
func (d *DB) TableCounts(ctx context.Context, tables []string) map[string]int64 {
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		row := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table)
		if err := row.Scan(&count); err == nil {
			counts[table] = count
		}
	}
	return counts
}

// IntegrityCheck runs PRAGMA integrity_check and returns the result.
// A healthy database returns "ok".
func (d *DB) IntegrityCheck(ctx context.Context) (string, error) {
	var result string
	row := d.QueryRowContext(ctx, "PRAGMA integrity_check")
	if err := row.Scan(&result); err != nil {
		return "", fmt.Errorf("db: integrity check: %w", err)
	}
	return result, nil
}

// timedRow wraps sql.Row to log after Scan completes.
type timedRow struct {
	row   *sql.Row
	db    *DB
	ctx   context.Context
	op    string
	query string
	args  []any
	start time.Time
}

// Scan reads the row and logs the query timing.
func (r *timedRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	r.db.logQuery(r.ctx, r.op, r.query, r.args, time.Since(r.start), err)
	return err
}

// Tx wraps sql.Tx with logging.
type Tx struct {
	tx    *sql.Tx
	db    *DB
	start time.Time
	ctx   context.Context
}

// ExecContext executes a query within the transaction.
func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := t.tx.ExecContext(ctx, query, args...)
	t.db.logQuery(ctx, "tx_exec", query, args, time.Since(start), err)
	return result, err
}

// QueryContext executes a query within the transaction.
func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := t.tx.QueryContext(ctx, query, args...)
	t.db.logQuery(ctx, "tx_query", query, args, time.Since(start), err)
	return rows, err
}

// QueryRowContext executes a query that returns one row within the transaction.
func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *timedRow {
	start := time.Now()
	row := t.tx.QueryRowContext(ctx, query, args...)
	return &timedRow{
		row:   row,
		db:    t.db,
		ctx:   ctx,
		op:    "tx_query_row",
		query: query,
		args:  args,
		start: start,
	}
}

// Commit commits the transaction with logging.
func (t *Tx) Commit() error {
	err := t.tx.Commit()
	duration := time.Since(t.start)
	if t.db.devMode {
		t.db.logger.Debug("sql_commit",
			"duration_ms", duration.Milliseconds(),
			"error", err,
			"component", "db",
		)
	}
	if err != nil {
		t.db.logger.Error("sql_commit_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"duration_ms", duration.Milliseconds(),
			"component", "db",
		)
	}
	return err
}

// Rollback rolls back the transaction with logging.
func (t *Tx) Rollback() error {
	err := t.tx.Rollback()
	duration := time.Since(t.start)
	if t.db.devMode {
		t.db.logger.Debug("sql_rollback",
			"duration_ms", duration.Milliseconds(),
			"error", err,
			"component", "db",
		)
	}
	return err
}
