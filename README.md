# wgpilot

WireGuard management tool with an embedded web UI. Single binary, no external dependencies.

## Features

- **Single binary** — Go server with embedded React SPA, SQLite database, TLS termination
- **Web dashboard** — Real-time peer status via SSE, network/peer CRUD, QR code config generation
- **Multi-network** — Manage multiple WireGuard interfaces with independent subnets and routing modes (gateway, site-to-site, hub-routed)
- **Automatic IP allocation** — Concurrent-safe IP assignment from configured subnets
- **nftables bridging** — Cross-network forwarding rules managed via kernel nftables API
- **Kernel-native** — Uses wgctrl-go and netlink directly (no `wg`/`ip`/`iptables` shell-outs)
- **Reconciliation** — On startup, reconciles kernel WireGuard state against the database
- **Setup wizard** — Browser-based 4-step setup with one-time password bootstrap
- **TLS** — ACME (Let's Encrypt), self-signed, or manual certificate modes
- **Self-updater** — Check for and apply updates from GitHub releases
- **Monitoring** — Peer polling, snapshot history, metrics endpoint, diagnostic CLI
- **Security** — JWT auth, bcrypt passwords, rate-limited login, security headers, encrypted private key storage

## Quick Start

### Install

```bash
# Download the latest release
curl -fsSL https://github.com/itsChris/wgpilot/releases/latest/download/wgpilot-linux-amd64 -o /usr/local/bin/wgpilot
chmod +x /usr/local/bin/wgpilot
```

### Initialize

```bash
# Create data directory and initialize the database
sudo wgpilot init --data-dir /var/lib/wgpilot
```

This generates a one-time setup password. Save it — you'll need it to complete setup via the web UI.

### Run

```bash
# Start the server
sudo wgpilot serve
```

Open `https://<server-ip>` in your browser and complete the setup wizard using the one-time password.

### Systemd Service

```ini
[Unit]
Description=wgpilot WireGuard Manager
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/wgpilot serve --config /etc/wgpilot/config.yaml
AmbientCapabilities=CAP_NET_ADMIN
WatchdogSec=60
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## CLI Reference

```
wgpilot serve       Start the HTTPS server
wgpilot init        Initialize database and generate setup credentials
wgpilot diagnose    Run system diagnostics (--json for machine output)
wgpilot update      Update to the latest release (--check for dry run)
wgpilot version     Print version, commit, and build date
wgpilot backup      Create a database backup
wgpilot restore     Restore database from a backup
wgpilot config check  Validate configuration file
```

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `/etc/wgpilot/config.yaml` | Path to config file |
| `--data-dir` | `/var/lib/wgpilot` | Path to data directory |
| `--log-level` | `info` | Log level: debug, info, warn, error |
| `--dev-mode` | `false` | Enable development mode (debug logging, relaxed TLS) |

## Configuration Reference

Configuration is loaded with priority: CLI flags > environment variables > YAML file > defaults.

Environment variables use the `WGPILOT_` prefix with underscores replacing dots (e.g., `WGPILOT_SERVER_LISTEN`).

```yaml
server:
  listen: "0.0.0.0:443"       # Listen address
  dev_mode: false              # Development mode

database:
  path: "/var/lib/wgpilot/wgpilot.db"

auth:
  session_ttl: "24h"           # JWT session lifetime
  bcrypt_cost: 12              # Password hashing cost
  rate_limit_rpm: 5            # Login attempts per minute per IP

tls:
  mode: "self-signed"          # self-signed | acme | manual
  acme_email: ""               # Required for ACME mode
  acme_domain: ""              # Required for ACME mode
  cert_file: ""                # Required for manual mode
  key_file: ""                 # Required for manual mode

logging:
  level: "info"                # debug | info | warn | error
  format: "json"               # Log output format

monitor:
  poll_interval: "30s"         # Peer status polling interval
  snapshot_retention: "30d"    # How long to keep peer snapshots
  compaction_interval: "24h"   # Snapshot compaction frequency
```

## Build from Source

Requirements: Go 1.24+, Node.js 18+

```bash
git clone https://github.com/itsChris/wgpilot.git
cd wgpilot

# Build frontend and Go binary
make build

# Or step by step:
cd frontend && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -ldflags="-s -w" -o wgpilot ./cmd/wgpilot

# Run tests
make test
make lint
```

## License

MIT
