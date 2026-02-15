-- +goose Up

ALTER TABLE peers ADD COLUMN expires_at INTEGER;

-- +goose Down

-- SQLite doesn't support DROP COLUMN before 3.35.0, so no down migration.
