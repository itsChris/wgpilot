# Data Model

> **Purpose**: Defines all SQLite tables, entity relationships, indexes, and known enum values.
>
> **Related docs**: [api-surface.md](api-surface.md), [../features/network-management.md](../features/network-management.md), [../features/peer-management.md](../features/peer-management.md), [../features/monitoring.md](../features/monitoring.md)
>
> **Implements**: `internal/db/`, `internal/db/migrations/001_initial.sql`

---

## Entity Relationship

```
Settings (key-value)
    │
    │
Network ──────< Peer
    │              │
    │              └── PeerSnapshot (time series)
    │
    └──< NetworkBridge >── Network
```

## Tables

### `settings`

Global configuration stored as key-value pairs.

```sql
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Known keys:
-- setup_complete       (bool)    "true" / "false"
-- public_ip            (string)  auto-detected external IP
-- hostname             (string)  optional, enables ACME TLS
-- default_dns          (string)  comma-separated DNS servers
-- admin_password_hash  (string)  bcrypt hash (set during setup)
-- install_token_hash   (string)  bcrypt hash of one-time install password
-- tls_mode             (string)  "self-signed" | "acme" | "manual"
-- smtp_host            (string)  for alert emails
-- smtp_port            (string)
-- smtp_user            (string)
-- smtp_pass            (string)  encrypted at rest
-- smtp_from            (string)
-- alert_email          (string)  recipient for alerts
```

### `users`

```sql
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'admin',  -- 'admin' only for now
    created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at    INTEGER NOT NULL DEFAULT (unixepoch())
);
```

### `networks`

```sql
CREATE TABLE networks (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    name                TEXT    NOT NULL,
    interface           TEXT    NOT NULL UNIQUE,       -- wg0, wg1, ...
    mode                TEXT    NOT NULL,              -- 'gateway' | 'site-to-site' | 'hub-routed'
    subnet              TEXT    NOT NULL,              -- CIDR: 10.0.0.0/24
    listen_port         INTEGER NOT NULL,
    private_key         TEXT    NOT NULL,              -- encrypted at rest
    public_key          TEXT    NOT NULL,
    dns_servers         TEXT    NOT NULL DEFAULT '',   -- comma-separated
    nat_enabled         BOOLEAN NOT NULL DEFAULT 0,
    inter_peer_routing  BOOLEAN NOT NULL DEFAULT 0,
    enabled             BOOLEAN NOT NULL DEFAULT 1,
    created_at          INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at          INTEGER NOT NULL DEFAULT (unixepoch()),

    UNIQUE(listen_port)
);
```

### `peers`

```sql
CREATE TABLE peers (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id            INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    name                  TEXT    NOT NULL,
    email                 TEXT    NOT NULL DEFAULT '',  -- optional, for identification
    private_key           TEXT    NOT NULL,             -- encrypted at rest, needed for config generation
    public_key            TEXT    NOT NULL,
    preshared_key         TEXT    NOT NULL DEFAULT '',  -- encrypted at rest, optional
    allowed_ips           TEXT    NOT NULL,             -- single IP/32 for clients, subnet for site gateways
    endpoint              TEXT    NOT NULL DEFAULT '',  -- null/empty for clients behind NAT
    persistent_keepalive  INTEGER NOT NULL DEFAULT 0,   -- seconds, 0 = disabled
    role                  TEXT    NOT NULL DEFAULT 'client',  -- 'client' | 'site-gateway'
    site_networks         TEXT    NOT NULL DEFAULT '',  -- additional CIDRs (site-to-site)
    enabled               BOOLEAN NOT NULL DEFAULT 1,
    created_at            INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at            INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX idx_peers_network ON peers(network_id);
```

### `network_bridges`

```sql
CREATE TABLE network_bridges (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    network_a_id    INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    network_b_id    INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    direction       TEXT    NOT NULL DEFAULT 'bidirectional',  -- 'a_to_b' | 'b_to_a' | 'bidirectional'
    allowed_cidrs   TEXT    NOT NULL DEFAULT '',  -- optional fine-grained filter
    enabled         BOOLEAN NOT NULL DEFAULT 1,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch()),

    UNIQUE(network_a_id, network_b_id)
);
```

### `peer_snapshots`

```sql
CREATE TABLE peer_snapshots (
    peer_id    INTEGER NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
    timestamp  INTEGER NOT NULL,  -- unix epoch
    rx_bytes   INTEGER NOT NULL,
    tx_bytes   INTEGER NOT NULL,
    online     BOOLEAN NOT NULL,

    PRIMARY KEY (peer_id, timestamp)
);

CREATE INDEX idx_snapshots_time ON peer_snapshots(peer_id, timestamp);
```

### `audit_log`

```sql
CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp  INTEGER NOT NULL DEFAULT (unixepoch()),
    user_id    INTEGER,           -- NULL for system events
    action     TEXT    NOT NULL,  -- 'peer.created', 'network.updated', etc.
    resource   TEXT    NOT NULL,  -- 'network:1', 'peer:5', etc.
    detail     TEXT    NOT NULL DEFAULT '',  -- JSON payload of changes
    ip_address TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_action ON audit_log(action);
```

### `alerts`

```sql
CREATE TABLE alerts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT    NOT NULL,  -- 'peer_offline' | 'interface_down' | 'transfer_spike'
    threshold  TEXT    NOT NULL,  -- '10m', '1GB/hour', etc.
    notify     TEXT    NOT NULL DEFAULT 'email',
    enabled    BOOLEAN NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
```
