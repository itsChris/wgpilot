-- +goose Up

CREATE TABLE api_keys (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    key_hash    TEXT    NOT NULL UNIQUE,
    key_prefix  TEXT    NOT NULL,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT    NOT NULL DEFAULT 'admin',
    expires_at  INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    last_used   INTEGER
);

CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);

-- +goose Down

DROP TABLE IF EXISTS api_keys;
