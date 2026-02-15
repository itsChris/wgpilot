<p align="center">
  <strong>wgpilot</strong><br>
  Self-hosted WireGuard management with a modern web UI.<br>
  Single binary. No shell-outs. No external dependencies.
</p>

<p align="center">
  <a href="https://github.com/itsChris/wgpilot/releases"><img src="https://img.shields.io/github/v/release/itsChris/wgpilot?style=flat-square" alt="Release"></a>
  <a href="https://github.com/itsChris/wgpilot/blob/master/LICENSE"><img src="https://img.shields.io/github/license/itsChris/wgpilot?style=flat-square" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/itsChris/wgpilot"><img src="https://goreportcard.com/badge/github.com/itsChris/wgpilot?style=flat-square" alt="Go Report Card"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" alt="Go 1.24+">
  <img src="https://img.shields.io/badge/WireGuard-kernel--native-88171A?style=flat-square&logo=wireguard" alt="WireGuard">
</p>

---

<!-- TODO: Add a screenshot of the dashboard here -->
<!-- ![wgpilot dashboard](docs/assets/screenshot-dashboard.png) -->

## Why wgpilot?

Most WireGuard management tools shell out to `wg`, `ip`, and `iptables` and parse their text output. wgpilot takes a different approach:

| | wgpilot | wg-easy | Firezone | Netbird |
|---|---|---|---|---|
| Shell-outs to wg/ip/iptables | **None** (kernel API) | Yes | Yes | Yes |
| Single binary | **Yes** | Docker only | Multi-container | Multi-container |
| Multi-network | **Yes** (3 topology modes) | No | Yes | Yes (mesh) |
| Network bridging | **Yes** | No | No | No |
| Encrypted key storage | **AES-256-GCM** | Plaintext | Depends | N/A |
| Self-contained DB | **SQLite** | JSON file | PostgreSQL | PostgreSQL |
| RBAC (multi-user) | **Yes** | No | Yes | Yes |
| API keys | **Yes** | No | Yes | Yes |
| Audit log | **Yes** | No | Yes | Yes |
| Setup effort | **curl + init** | docker run | Docker + DB | Docker + DB + STUN |

wgpilot is ideal if you want a **lightweight, secure, self-contained** WireGuard manager that runs natively on Linux without Docker or external databases.

## Features

### Core
- **Single binary** -- Go server + embedded React SPA + SQLite + TLS termination
- **Kernel-native** -- Uses `wgctrl-go`, `netlink`, and `nftables` directly (zero shell-outs)
- **Reconciliation** -- On startup, reconciles kernel WireGuard state against the database
- **Setup wizard** -- Browser-based 4-step setup with one-time password bootstrap

### Networking
- **Multi-network** -- Manage multiple WireGuard interfaces with independent subnets
- **3 topology modes** -- VPN gateway, site-to-site, and hub-routed (see [Topologies](#topologies))
- **Network bridging** -- Cross-network forwarding rules via nftables kernel API
- **Automatic IP allocation** -- Concurrent-safe IP assignment from configured subnets
- **Config export** -- Download wg-quick compatible server configs
- **wg-quick import** -- Import existing WireGuard configurations during setup

### Security
- **JWT auth** with HttpOnly/Secure/SameSite cookies
- **Multi-user RBAC** -- Admin and viewer roles
- **API keys** -- Bearer token auth for automation (`wgp_...` prefix)
- **Encrypted private keys** -- AES-256-GCM at rest, derived from JWT secret
- **Rate-limited login** -- 5 attempts per minute per IP
- **Audit log** -- Every mutating operation logged with user, IP, and timestamp
- **Security headers** -- CSP, HSTS, X-Frame-Options, X-Content-Type-Options

### Monitoring
- **Real-time dashboard** -- Live peer status via Server-Sent Events (SSE)
- **Prometheus metrics** -- `wg_peers_total`, `wg_transfer_bytes_total`, `wg_peer_last_handshake_seconds`, etc.
- **Alert rules** -- Configurable alerts for peer offline, interface down
- **Transfer history** -- Historical RX/TX data with automatic compaction
- **Diagnostic CLI** -- `wgpilot diagnose` for system health checks

### Operations
- **TLS** -- ACME (Let's Encrypt), self-signed, or manual certificates
- **Self-updater** -- Check and apply updates from GitHub releases
- **Backup/restore** -- CLI commands for database backup and recovery
- **Peer expiry** -- Optional expiration dates with automatic peer disable
- **Docker support** -- Dockerfile + docker-compose included
- **systemd integration** -- Type=notify with watchdog support

## Architecture

```mermaid
graph TB
    subgraph "Single Binary"
        SPA["React SPA<br/>(embedded via go:embed)"]
        API["REST API<br/>(net/http)"]
        AUTH["Auth Middleware<br/>(JWT + API Keys)"]
        WG["WireGuard Manager<br/>(wgctrl-go)"]
        NL["Netlink Manager<br/>(vishvananda/netlink)"]
        NFT["Firewall Manager<br/>(google/nftables)"]
        DB["SQLite<br/>(modernc.org/sqlite)"]
        MON["Monitor<br/>(SSE + Prometheus)"]
    end

    USER["Browser / API Client"] -->|HTTPS| SPA
    USER -->|HTTPS| API
    API --> AUTH
    AUTH --> API

    API --> WG
    API --> NFT
    API --> DB
    API --> MON

    WG -->|"Netlink Socket"| KERNEL["Linux Kernel<br/>WireGuard Module"]
    NL -->|"Netlink Socket"| KERNEL
    NFT -->|"Netlink Socket"| KERNEL

    DB -->|"WAL mode"| SQLITE[("wgpilot.db")]
    MON -->|SSE| USER

    style SPA fill:#61dafb,color:#000
    style KERNEL fill:#88171a,color:#fff
    style SQLITE fill:#003B57,color:#fff
```

### Request Flow

```mermaid
sequenceDiagram
    participant Client
    participant Middleware
    participant Handler
    participant WireGuard
    participant Database
    participant Kernel

    Client->>Middleware: POST /api/networks/{id}/peers
    Middleware->>Middleware: Validate JWT / API Key
    Middleware->>Handler: Authenticated request

    Handler->>Handler: Parse & validate input
    Handler->>Database: Insert peer record
    Database-->>Handler: Peer ID

    Handler->>WireGuard: AddPeer(publicKey, allowedIPs)
    WireGuard->>Kernel: Netlink: configure peer
    Kernel-->>WireGuard: OK

    Handler->>Database: Write audit log
    Handler-->>Client: 201 Created + peer JSON
```

## Topologies

wgpilot supports three network topology modes out of the box:

### VPN Gateway (Remote Access)

```mermaid
graph LR
    subgraph "Internet"
        LAPTOP["Laptop<br/>10.0.0.2"]
        PHONE["Phone<br/>10.0.0.3"]
        TABLET["Tablet<br/>10.0.0.4"]
    end

    subgraph "Server (wgpilot)"
        GW["wg0 Gateway<br/>10.0.0.1<br/>NAT Masquerade"]
    end

    subgraph "Internet / LAN"
        WEB["Web"]
        LAN["Internal Services"]
    end

    LAPTOP -->|"WireGuard Tunnel"| GW
    PHONE -->|"WireGuard Tunnel"| GW
    TABLET -->|"WireGuard Tunnel"| GW
    GW -->|"NAT"| WEB
    GW -->|"Forward"| LAN

    style GW fill:#88171a,color:#fff
```

All client traffic routed through the server with NAT masquerading. Ideal for remote access VPN and privacy tunnels.

### Site-to-Site

```mermaid
graph LR
    subgraph "Office A — 192.168.1.0/24"
        A_PC["Workstations"]
        A_GW["Site A Gateway<br/>10.0.0.2"]
    end

    subgraph "wgpilot Server"
        HUB["wg0 Hub<br/>10.0.0.1"]
    end

    subgraph "Office B — 192.168.2.0/24"
        B_GW["Site B Gateway<br/>10.0.0.3"]
        B_SRV["Servers"]
    end

    A_PC --- A_GW
    A_GW -->|"Tunnel"| HUB
    HUB -->|"Tunnel"| B_GW
    B_GW --- B_SRV

    style HUB fill:#88171a,color:#fff
```

Each peer represents a site gateway. Traffic is routed between office LANs through the WireGuard hub.

### Hub with Peer Routing (Team Mesh)

```mermaid
graph TB
    subgraph "wgpilot Server"
        HUB["wg0 Hub<br/>10.0.0.1<br/>Inter-peer Forwarding"]
    end

    DEV1["Dev 1<br/>10.0.0.2"] -->|"Tunnel"| HUB
    DEV2["Dev 2<br/>10.0.0.3"] -->|"Tunnel"| HUB
    DEV3["Dev 3<br/>10.0.0.4"] -->|"Tunnel"| HUB

    DEV1 -.->|"via Hub"| DEV2
    DEV1 -.->|"via Hub"| DEV3
    DEV2 -.->|"via Hub"| DEV3

    style HUB fill:#88171a,color:#fff
```

Clients can reach each other through the hub server. No NAT, no direct internet routing. Ideal for development teams and internal networks.

### Network Bridging

```mermaid
graph LR
    subgraph "Network A — wg0"
        A1["Peer A1<br/>10.0.1.2"]
        A2["Peer A2<br/>10.0.1.3"]
    end

    subgraph "wgpilot Server"
        WG0["wg0<br/>10.0.1.1"]
        BRIDGE["Bridge Rule<br/>(nftables)"]
        WG1["wg1<br/>10.0.2.1"]
    end

    subgraph "Network B — wg1"
        B1["Peer B1<br/>10.0.2.2"]
        B2["Peer B2<br/>10.0.2.3"]
    end

    A1 --- WG0
    A2 --- WG0
    WG0 ---|"Controlled<br/>Forwarding"| BRIDGE
    BRIDGE ---|"Controlled<br/>Forwarding"| WG1
    WG1 --- B1
    WG1 --- B2

    style BRIDGE fill:#f59e0b,color:#000
```

Connect separate WireGuard networks with controlled forwarding rules. Supports unidirectional and bidirectional bridging with optional CIDR filtering.

## Quick Start

### Install

```bash
# Download the latest release
curl -fsSL https://github.com/itsChris/wgpilot/releases/latest/download/wgpilot-linux-amd64 \
  -o /usr/local/bin/wgpilot
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

### Docker

```bash
# Using docker-compose
git clone https://github.com/itsChris/wgpilot.git
cd wgpilot
docker compose up -d
```

Or with `docker run`:

```bash
docker run -d \
  --name wgpilot \
  --network host \
  --cap-add NET_ADMIN \
  --cap-add SYS_MODULE \
  --sysctl net.ipv4.ip_forward=1 \
  -v wgpilot-data:/var/lib/wgpilot \
  -v /etc/wgpilot:/etc/wgpilot \
  --restart unless-stopped \
  ghcr.io/itschris/wgpilot:latest
```

### systemd Service

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
wgpilot serve              Start the HTTPS server
wgpilot init               Initialize database and generate setup credentials
wgpilot diagnose           Run system diagnostics (--json for machine output)
wgpilot update             Update to the latest release (--check for dry run)
wgpilot version            Print version, commit, and build date
wgpilot backup             Create a database backup
wgpilot restore            Restore database from a backup
wgpilot config check       Validate configuration file
wgpilot api-key create     Create an API key (--name, --role, --expires-in)
wgpilot api-key list       List all API keys
wgpilot api-key revoke     Revoke an API key by ID
```

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `/etc/wgpilot/config.yaml` | Path to config file |
| `--data-dir` | `/var/lib/wgpilot` | Path to data directory |
| `--log-level` | `info` | Log level: debug, info, warn, error |
| `--dev-mode` | `false` | Enable development mode (debug logging, relaxed TLS) |

### API Keys

Create API keys for automation and scripting:

```bash
# Create an admin API key
sudo wgpilot api-key create --name "CI/CD" --role admin --expires-in 90d

# Create a read-only key
sudo wgpilot api-key create --name "monitoring" --role viewer

# List keys
sudo wgpilot api-key list

# Revoke a key
sudo wgpilot api-key revoke <key-id>
```

Use API keys with the `Authorization` header:

```bash
curl -H "Authorization: Bearer wgp_abc123..." https://your-server/api/networks
```

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

## Roadmap

See [docs/roadmap-v0.3.0.md](docs/roadmap-v0.3.0.md) for the full v0.3.0 plan.

### Planned (v0.3.0)

- **Per-peer bandwidth limits** -- QoS via HTB qdiscs (upload/download per peer)
- **Split-tunnel / policy routing** -- Managed ip rules, routing tables, fwmark
- **Interface-level statistics** -- RX/TX/errors/drops counters + Prometheus metrics
- **Event-driven monitoring** -- Instant detection via netlink subscriptions (replace polling)
- **MTU management** -- Per-network MTU configuration
- **Active connection viewer** -- See live TCP/UDP flows through the VPN (conntrack)
- **Port conflict detection** -- Clear errors when a port is already in use
- **Route table viewer** -- Kernel routing tables and ip rules in the web UI

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Follow the commit convention: `feat(scope): description`, `fix(scope): description`
4. Run `make test && make lint` before submitting
5. Open a pull request against `master`

See [CLAUDE.md](CLAUDE.md) for detailed coding conventions, architecture rules, and package layout.

## Acknowledgements

Built on these excellent Go libraries:

- [wgctrl-go](https://github.com/WireGuard/wgctrl-go) -- WireGuard kernel API
- [vishvananda/netlink](https://github.com/vishvananda/netlink) -- Network interface management
- [google/nftables](https://github.com/google/nftables) -- Firewall rule management
- [modernc.org/sqlite](https://modernc.org/sqlite) -- Pure Go SQLite

## License

[MIT](LICENSE)
