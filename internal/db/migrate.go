package db

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all embedded SQL migration files against the database.
// Migrations are tracked in a _migrations table and only applied once.
func Migrate(ctx context.Context, d *DB, logger *slog.Logger) error {
	// Create migrations tracking table.
	_, err := d.conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _migrations (
			filename TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT (unixepoch())
		)
	`)
	if err != nil {
		return fmt.Errorf("db: create migrations table: %w", err)
	}

	// Read all migration files.
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("db: read migrations dir: %w", err)
	}

	// Sort by filename for deterministic ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Check if already applied.
		var count int
		err := d.conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM _migrations WHERE filename = ?",
			entry.Name(),
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("db: check migration %s: %w", entry.Name(), err)
		}
		if count > 0 {
			logger.Debug("migration_skipped",
				"filename", entry.Name(),
				"reason", "already_applied",
				"component", "db",
			)
			continue
		}

		// Read and execute the migration.
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("db: read migration %s: %w", entry.Name(), err)
		}

		sql := string(content)

		// Strip goose directives if present, only run the Up portion.
		sql = extractUpSection(sql)

		if _, err := d.conn.ExecContext(ctx, sql); err != nil {
			return fmt.Errorf("db: apply migration %s: %w", entry.Name(), err)
		}

		// Record it.
		if _, err := d.conn.ExecContext(ctx,
			"INSERT INTO _migrations (filename) VALUES (?)",
			entry.Name(),
		); err != nil {
			return fmt.Errorf("db: record migration %s: %w", entry.Name(), err)
		}

		logger.Info("migration_applied",
			"filename", entry.Name(),
			"component", "db",
		)
	}

	return nil
}

// extractUpSection returns only the SQL between "-- +goose Up" and "-- +goose Down".
// If no goose directives are found, returns the full content.
func extractUpSection(sql string) string {
	upIdx := strings.Index(sql, "-- +goose Up")
	downIdx := strings.Index(sql, "-- +goose Down")

	if upIdx == -1 {
		return sql
	}

	start := upIdx + len("-- +goose Up")
	if downIdx == -1 {
		return strings.TrimSpace(sql[start:])
	}

	return strings.TrimSpace(sql[start:downIdx])
}
