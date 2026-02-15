# Service Management

> **Purpose**: Specifies the systemd unit file, Linux capabilities, filesystem layout, CLI subcommands, configuration file format, signal handling, and lifecycle management.
>
> **Related docs**: [../features/install-script.md](../features/install-script.md), [tls.md](tls.md), [../architecture/project-structure.md](../architecture/project-structure.md)
>
> **Implements**: `cmd/wgpilot/main.go`, `internal/config/`, systemd unit file (installed by `install.sh`)

---

## Systemd Unit

Hardened unit file with minimal privileges:

```ini
[Unit]
Description=WireGuard Web UI
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/wgpilot serve
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
WatchdogSec=30

User=wg-webui
Group=wg-webui

AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=true

ReadWritePaths=/var/lib/wg-webui
ReadOnlyPaths=/etc/wg-webui
ProtectHome=true
ProtectSystem=strict
PrivateTmp=true

RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_UNIX
ProtectKernelTunables=false
ProtectKernelModules=true
ProtectKernelLogs=true
LockPersonality=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native

Environment=WG_WEBUI_DATA_DIR=/var/lib/wg-webui
Environment=WG_WEBUI_CONFIG=/etc/wg-webui/config.yaml
Environment=WG_WEBUI_LOG_LEVEL=info

[Install]
WantedBy=multi-user.target
```

## Capabilities

| Capability | Purpose |
|---|---|
| `CAP_NET_ADMIN` | Create/configure WireGuard interfaces, manage routes, nftables |
| `CAP_NET_BIND_SERVICE` | Bind to port 443 without root |

## Filesystem Layout

```
/usr/local/bin/wgpilot              # binary
/etc/wg-webui/config.yaml         # static config (read-only by service)
/var/lib/wg-webui/
├── data.db                       # SQLite database
├── data.db-wal                   # WAL journal
└── certs/                        # auto-TLS certificates
```

## CLI Subcommands

The binary serves as both the long-running service and a management CLI:

```
wgpilot serve                       # start the HTTP server (systemd calls this)
wgpilot init                        # first-time setup: create DB, generate install token
    --admin-pass=STRING           # set the one-time install token
    --data-dir=PATH               # database directory
wgpilot config check                # validate config.yaml
wgpilot status                      # show running networks, peer count, uptime
wgpilot backup                      # dump SQLite to stdout (pipe to file)
    --output=PATH                 # write to file instead of stdout
wgpilot restore <file>              # restore database from backup file
wgpilot update                      # self-update from GitHub releases
    --check                       # just check for updates, don't install
    --version=STRING              # install specific version
wgpilot version                     # print version, commit, build date
```

All CLI commands (except `serve`) are short-lived operations that either talk to the database directly or query the running API.

## Configuration File

```yaml
# /etc/wg-webui/config.yaml

listen: 0.0.0.0:443

data_dir: /var/lib/wg-webui

tls:
  mode: self-signed              # self-signed | acme | manual
  domain: ""                     # required for acme mode
  cert_file: ""                  # required for manual mode
  key_file: ""                   # required for manual mode

auth:
  session_ttl: 24h
  bcrypt_cost: 12

log:
  level: info                    # debug | info | warn | error
  format: json                   # json | text
```

## Configuration Precedence

1. CLI flags (highest priority)
2. Environment variables (`WG_WEBUI_*`)
3. Config file (`/etc/wg-webui/config.yaml`)
4. Defaults (lowest priority)

## Environment Variable Mapping

```
WG_WEBUI_LISTEN       → listen
WG_WEBUI_DATA_DIR     → data_dir
WG_WEBUI_TLS_MODE     → tls.mode
WG_WEBUI_TLS_DOMAIN   → tls.domain
WG_WEBUI_LOG_LEVEL    → log.level
WG_WEBUI_LOG_FORMAT   → log.format
```

## Signal Handling

```
SIGTERM/SIGINT → graceful shutdown
    ├── Stop accepting connections
    ├── Finish in-flight requests (10s timeout)
    ├── Stop monitoring poller
    ├── Close database
    └── Exit 0

SIGHUP → reload config
    ├── Re-read config.yaml
    ├── Update log level
    └── Refresh TLS certificates
```
