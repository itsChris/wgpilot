-- +goose Up

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'admin',
    created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at    INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE networks (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    name                TEXT    NOT NULL,
    interface           TEXT    NOT NULL UNIQUE,
    mode                TEXT    NOT NULL,
    subnet              TEXT    NOT NULL,
    listen_port         INTEGER NOT NULL,
    private_key         TEXT    NOT NULL,
    public_key          TEXT    NOT NULL,
    dns_servers         TEXT    NOT NULL DEFAULT '',
    nat_enabled         BOOLEAN NOT NULL DEFAULT 0,
    inter_peer_routing  BOOLEAN NOT NULL DEFAULT 0,
    enabled             BOOLEAN NOT NULL DEFAULT 1,
    created_at          INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at          INTEGER NOT NULL DEFAULT (unixepoch()),

    UNIQUE(listen_port)
);

CREATE TABLE peers (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id            INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    name                  TEXT    NOT NULL,
    email                 TEXT    NOT NULL DEFAULT '',
    private_key           TEXT    NOT NULL,
    public_key            TEXT    NOT NULL,
    preshared_key         TEXT    NOT NULL DEFAULT '',
    allowed_ips           TEXT    NOT NULL,
    endpoint              TEXT    NOT NULL DEFAULT '',
    persistent_keepalive  INTEGER NOT NULL DEFAULT 0,
    role                  TEXT    NOT NULL DEFAULT 'client',
    site_networks         TEXT    NOT NULL DEFAULT '',
    enabled               BOOLEAN NOT NULL DEFAULT 1,
    created_at            INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at            INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX idx_peers_network ON peers(network_id);

CREATE TABLE peer_snapshots (
    peer_id    INTEGER NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
    timestamp  INTEGER NOT NULL,
    rx_bytes   INTEGER NOT NULL,
    tx_bytes   INTEGER NOT NULL,
    online     BOOLEAN NOT NULL,

    PRIMARY KEY (peer_id, timestamp)
);

CREATE INDEX idx_snapshots_time ON peer_snapshots(peer_id, timestamp);

CREATE TABLE network_bridges (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    network_a_id    INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    network_b_id    INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    direction       TEXT    NOT NULL DEFAULT 'bidirectional',
    allowed_cidrs   TEXT    NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT 1,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch()),

    UNIQUE(network_a_id, network_b_id)
);

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp  INTEGER NOT NULL DEFAULT (unixepoch()),
    user_id    INTEGER,
    action     TEXT    NOT NULL,
    resource   TEXT    NOT NULL,
    detail     TEXT    NOT NULL DEFAULT '',
    ip_address TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_action ON audit_log(action);

CREATE TABLE alerts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT    NOT NULL,
    threshold  TEXT    NOT NULL,
    notify     TEXT    NOT NULL DEFAULT 'email',
    enabled    BOOLEAN NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- +goose Down

DROP TABLE IF EXISTS alerts;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS network_bridges;
DROP TABLE IF EXISTS peer_snapshots;
DROP TABLE IF EXISTS peers;
DROP TABLE IF EXISTS networks;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS settings;
