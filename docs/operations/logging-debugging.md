# Logging, Debugging & Diagnostics Specification

> **Purpose**: Define a two-tier logging strategy (production and development) and comprehensive diagnostics infrastructure that gives Claude Code (or any developer) maximum visibility into what went wrong, where, and why — across every layer of the application.
>
> **Related docs**: [service.md](../operations/service.md), [tech-stack.md](../architecture/tech-stack.md), [api-surface.md](../architecture/api-surface.md)
>
> **Implements**: `internal/logging/`, `internal/middleware/`, `internal/debug/`, CLI flags `--log-level`, `--dev-mode`

---

## Two Logging Modes

### Production Mode (default)

Log level: `INFO`. Structured JSON to stdout. Captured by journald. Designed for operational awareness — what happened, not how.

### Development Mode (`--dev-mode` or `WG_LOG_LEVEL=debug`)

Log level: `DEBUG`. Adds: full request/response bodies, SQL queries with parameters, WireGuard kernel calls and responses, nftables rule changes, timing for every operation, goroutine and memory snapshots. Designed for Claude Code to trace a bug from symptom to root cause without asking "can you add more logging?"

---

## Logging Framework

Use `log/slog` from the Go standard library. No third-party logging frameworks.

```go
package logging

import (
    "log/slog"
    "os"
)

type Config struct {
    Level      slog.Level
    DevMode    bool
    AddSource  bool // include file:line in every log entry
}

func New(cfg Config) *slog.Logger {
    opts := &slog.HandlerOptions{
        Level:     cfg.Level,
        AddSource: cfg.AddSource || cfg.DevMode, // always add source in dev mode
    }

    var handler slog.Handler
    if cfg.DevMode {
        // human-readable, colorized output for terminal
        handler = slog.NewTextHandler(os.Stdout, opts)
    } else {
        // structured JSON for journald / log aggregators
        handler = slog.NewJSONHandler(os.Stdout, opts)
    }

    return slog.New(handler)
}
```

Every package receives the logger via dependency injection. No global logger. No `slog.Default()` calls outside of `main()`.

---

## What Gets Logged at Each Level

### ERROR — something broke, needs attention

Every ERROR log entry MUST include:

- `error`: the Go error string
- `error_type`: the type name of the error (e.g., `*net.OpError`, `*sqlite.Error`)
- `stack`: abbreviated stack trace (goroutine + top 5 frames) in dev mode
- `operation`: what the code was trying to do (e.g., `create_interface`, `add_peer`)
- `component`: which subsystem (e.g., `wg`, `nft`, `db`, `http`, `auth`)
- Any relevant entity IDs (`network_id`, `peer_id`)

```go
logger.Error("failed to create wireguard interface",
    "error", err,
    "error_type", fmt.Sprintf("%T", err),
    "operation", "create_interface",
    "component", "wg",
    "network_id", network.ID,
    "interface", network.Interface,
    "listen_port", network.ListenPort,
)
```

### WARN — something unexpected but recovered

- Peer offline longer than threshold
- ACME cert renewal retry
- Config file permission issues (readable but wrong mode)
- Database migration applied
- Kernel state mismatch during reconciliation (auto-corrected)

### INFO — operational events (production baseline)

- Service started / stopped / reloaded
- Network created / updated / deleted
- Peer created / updated / deleted / enabled / disabled
- Peer came online / went offline
- Admin login / logout / failed login attempt
- TLS certificate provisioned / renewed
- Self-update started / completed
- Backup created / restored
- Setup wizard completed

Each INFO entry includes: `timestamp`, `msg`, `component`, and relevant entity IDs. No request bodies, no SQL, no verbose detail.

### DEBUG — everything, for tracing (dev mode only)

This is where Claude Code gets its diagnostic power. DEBUG logs every internal decision and data flow.

---

## Per-Layer Debug Logging

### HTTP Layer

Log every request and response. In dev mode, include bodies.

```go
func RequestLoggingMiddleware(logger *slog.Logger, devMode bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            requestID := generateRequestID()

            // Always log
            attrs := []any{
                "request_id", requestID,
                "method", r.Method,
                "path", r.URL.Path,
                "remote_addr", r.RemoteAddr,
                "user_agent", r.UserAgent(),
            }

            // Dev mode: log request body
            if devMode && r.Body != nil && r.ContentLength > 0 && r.ContentLength < 1_000_000 {
                body, _ := io.ReadAll(r.Body)
                r.Body = io.NopCloser(bytes.NewReader(body))
                attrs = append(attrs, "request_body", string(body))
            }

            // Capture response
            wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
            
            // Add request ID to context for downstream correlation
            ctx := context.WithValue(r.Context(), requestIDKey, requestID)
            next.ServeHTTP(wrapped, r.WithContext(ctx))

            duration := time.Since(start)

            attrs = append(attrs,
                "status", wrapped.statusCode,
                "duration_ms", duration.Milliseconds(),
                "bytes_written", wrapped.bytesWritten,
            )

            // Dev mode: log response body for non-200 or for all
            if devMode && wrapped.body.Len() > 0 && wrapped.body.Len() < 1_000_000 {
                attrs = append(attrs, "response_body", wrapped.body.String())
            }

            // Auth context if available
            if user := authFromContext(r.Context()); user != nil {
                attrs = append(attrs, "user", user.Username)
            }

            level := slog.LevelInfo
            if wrapped.statusCode >= 500 {
                level = slog.LevelError
            } else if wrapped.statusCode >= 400 {
                level = slog.LevelWarn
            }

            logger.Log(r.Context(), level, "http_request", attrs...)
        })
    }
}
```

Every HTTP response with status >= 400 must include a JSON error body with a `request_id` field so logs can be correlated:

```json
{
    "error": "peer not found",
    "request_id": "req_abc123",
    "code": "PEER_NOT_FOUND"
}
```

### Database Layer

Wrap every query with timing and parameter logging in dev mode.

```go
type DB struct {
    conn    *sql.DB
    logger  *slog.Logger
    devMode bool
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
    start := time.Now()
    requestID, _ := ctx.Value(requestIDKey).(string)

    rows, err := db.conn.QueryContext(ctx, query, args...)
    duration := time.Since(start)

    if db.devMode {
        db.logger.Debug("sql_query",
            "request_id", requestID,
            "query", query,
            "args", fmt.Sprintf("%v", args),
            "duration_ms", duration.Milliseconds(),
            "error", err,
        )
    }

    if err != nil {
        db.logger.Error("sql_query_failed",
            "request_id", requestID,
            "query", query,
            "error", err,
            "error_type", fmt.Sprintf("%T", err),
            "duration_ms", duration.Milliseconds(),
            "component", "db",
        )
    }

    // Warn on slow queries (even in production)
    if duration > 100*time.Millisecond {
        db.logger.Warn("slow_query",
            "request_id", requestID,
            "query", query,
            "duration_ms", duration.Milliseconds(),
            "component", "db",
        )
    }

    return rows, err
}
```

In dev mode, also log:
- Schema migrations applied (with before/after version)
- Row counts affected by INSERT/UPDATE/DELETE
- Transaction start/commit/rollback with duration

### WireGuard Management Layer

Log every kernel interaction. This is the most critical layer for debugging because failures here are often silent or return cryptic netlink errors.

```go
func (m *Manager) CreateInterface(name string, addr string, port int) error {
    m.logger.Debug("wg_create_interface_start",
        "interface", name,
        "address", addr,
        "listen_port", port,
        "component", "wg",
    )

    // Step 1: Create link
    la := netlink.NewLinkAttrs()
    la.Name = name
    link := &netlink.Wireguard{LinkAttrs: la}
    if err := netlink.LinkAdd(link); err != nil {
        m.logger.Error("wg_link_add_failed",
            "interface", name,
            "error", err,
            "error_type", fmt.Sprintf("%T", err),
            "component", "wg",
            "hint", classifyNetlinkError(err),
        )
        return fmt.Errorf("create interface %s: %w", name, err)
    }
    m.logger.Debug("wg_link_added", "interface", name, "component", "wg")

    // Step 2: Assign address
    parsedAddr, err := netlink.ParseAddr(addr)
    if err != nil {
        m.logger.Error("wg_parse_addr_failed",
            "address", addr,
            "error", err,
            "component", "wg",
        )
        return fmt.Errorf("parse address %s: %w", addr, err)
    }
    if err := netlink.AddrAdd(link, parsedAddr); err != nil {
        m.logger.Error("wg_addr_add_failed",
            "interface", name,
            "address", addr,
            "error", err,
            "error_type", fmt.Sprintf("%T", err),
            "component", "wg",
            "hint", classifyNetlinkError(err),
        )
        return fmt.Errorf("assign address %s to %s: %w", addr, name, err)
    }
    m.logger.Debug("wg_addr_assigned", "interface", name, "address", addr, "component", "wg")

    // ... continue for key config, link up, etc.
    // Every step: log start, log success, log failure with full context

    m.logger.Info("wg_interface_created",
        "interface", name,
        "address", addr,
        "listen_port", port,
        "component", "wg",
    )
    return nil
}

// Translate cryptic netlink errors into actionable hints
func classifyNetlinkError(err error) string {
    msg := err.Error()
    switch {
    case strings.Contains(msg, "operation not permitted"):
        return "missing CAP_NET_ADMIN capability — check systemd unit AmbientCapabilities"
    case strings.Contains(msg, "file exists"):
        return "interface already exists — check for orphaned WG interfaces with 'ip link show'"
    case strings.Contains(msg, "no such device"):
        return "wireguard kernel module not loaded — run 'modprobe wireguard'"
    case strings.Contains(msg, "address already in use"):
        return "listen port already bound — check with 'ss -ulnp | grep <port>'"
    case strings.Contains(msg, "no buffer space available"):
        return "too many network interfaces — check 'ip link | wc -l'"
    default:
        return "unknown netlink error — check 'dmesg | tail -20' for kernel messages"
    }
}
```

### nftables Layer

Log every rule addition and removal with the full rule definition.

```go
func (n *NFTManager) AddNATRule(iface string, subnet string) error {
    n.logger.Debug("nft_add_nat_start",
        "interface", iface,
        "subnet", subnet,
        "component", "nft",
    )

    // ... add rule ...

    if n.devMode {
        // Dump current ruleset for debugging
        rules, _ := n.DumpRules()
        n.logger.Debug("nft_ruleset_after_change",
            "interface", iface,
            "action", "add_nat",
            "ruleset", rules,
            "component", "nft",
        )
    }

    return nil
}
```

### Auth Layer

Log every auth decision, but NEVER log passwords, tokens, or session secrets.

```go
// Good:
logger.Info("auth_login_success", "user", username, "remote_addr", remoteAddr, "component", "auth")
logger.Warn("auth_login_failed", "user", username, "remote_addr", remoteAddr, "reason", "invalid_password", "component", "auth")
logger.Warn("auth_token_expired", "user", username, "token_issued_at", issuedAt, "component", "auth")

// NEVER:
logger.Debug("auth_attempt", "user", username, "password", password) // NEVER LOG CREDENTIALS
```

---

## Request ID Correlation

Every incoming HTTP request gets a unique `request_id` (format: `req_<12 random hex chars>`). This ID propagates through the entire call chain via `context.Context`. Every log entry from that request — HTTP, DB, WG, nftables — includes the same `request_id`. This lets Claude Code trace a single API call across all layers:

```bash
# Trace everything that happened during a specific request
journalctl -u wgpilot --output=json | jq 'select(.request_id == "req_a1b2c3d4e5f6")'
```

For background operations (polling, compaction, reconciliation), use a `task_id` with format `task_<name>_<timestamp>`.

---

## Diagnostic Endpoint

`GET /api/debug/info` (admin-only, dev mode only)

Returns a complete system diagnostic snapshot:

```json
{
    "version": "0.3.1",
    "go_version": "go1.23.2",
    "os": "linux",
    "arch": "amd64",
    "kernel": "6.1.0-18-amd64",
    "uptime_seconds": 84231,
    "config": {
        "listen": "0.0.0.0:443",
        "data_dir": "/var/lib/wgpilot",
        "tls_mode": "self-signed",
        "log_level": "debug"
    },
    "database": {
        "path": "/var/lib/wgpilot/data.db",
        "size_bytes": 245760,
        "wal_size_bytes": 8192,
        "tables": {
            "networks": 2,
            "peers": 12,
            "peer_snapshots": 48320,
            "settings": 8
        },
        "schema_version": 5
    },
    "wireguard": {
        "kernel_module_loaded": true,
        "kernel_version": "built-in",
        "interfaces": [
            {
                "name": "wg0",
                "state": "up",
                "address": "10.0.0.1/24",
                "listen_port": 51820,
                "public_key": "abc...xyz=",
                "peer_count": 9,
                "peers_online": 7
            }
        ]
    },
    "system": {
        "ip_forward_v4": true,
        "ip_forward_v6": true,
        "nftables_loaded": true,
        "capabilities": ["CAP_NET_ADMIN", "CAP_NET_BIND_SERVICE"],
        "memory_mb": 42,
        "goroutines": 18,
        "open_files": 23,
        "cpu_count": 2
    },
    "network": {
        "public_ip": "203.0.113.45",
        "default_interface": "eth0",
        "default_gateway": "203.0.113.1"
    },
    "tls": {
        "mode": "self-signed",
        "cert_expiry": "2026-03-15T00:00:00Z",
        "cert_domains": []
    }
}
```

---

## Diagnostic CLI Subcommand

`wgpilot diagnose` — runs offline (doesn't need the service running). Outputs a full diagnostic report to stdout. This is what a user pastes into a GitHub issue or hands to Claude Code.

```bash
$ wgpilot diagnose

wgpilot diagnostic report
========================
Version:     0.3.1
Go:          go1.23.2
OS:          linux/amd64
Kernel:      6.1.0-18-amd64

[PASS] WireGuard kernel module loaded
[PASS] IP forwarding enabled (v4)
[PASS] IP forwarding enabled (v6)
[PASS] nftables available
[PASS] CAP_NET_ADMIN capability
[PASS] CAP_NET_BIND_SERVICE capability
[PASS] Data directory /var/lib/wgpilot exists and writable
[PASS] Database /var/lib/wgpilot/data.db accessible
[PASS] Database schema version: 5 (current)
[WARN] TLS certificate expires in 12 days
[PASS] Port 443 available
[PASS] Port 51820/udp available
[FAIL] Port 51821/udp in use by pid 1234 (wg-quick)

Interface wg0:
  State:       up
  Address:     10.0.0.1/24
  Listen port: 51820
  Peers:       9 configured, 7 online, 2 offline
  Transfer:    ↑312 GB ↓535 GB

Interface wg1:
  State:       up
  Address:     10.1.0.1/24
  Listen port: 51821
  Peers:       1 configured, 1 online
  Transfer:    ↑45 GB ↓82 GB

nftables rules:
  [2 NAT MASQUERADE rules]
  [1 FORWARD rule]

Recent errors (last 24h):
  2025-12-01 14:32:11 [ERROR] wg: peer handshake timeout peer="Dad's PC" network=wg0
  2025-12-01 08:15:03 [WARN]  db: slow query duration_ms=342 query="SELECT * FROM peer_snapshots..."

Database stats:
  Networks:       2
  Peers:          12
  Snapshots:      48,320 rows (2.1 MB)
  DB size:        240 KB
  WAL size:       8 KB
```

The `diagnose` command checks:

1. **Binary and runtime**: version, Go version, OS, architecture, kernel version
2. **Capabilities**: CAP_NET_ADMIN, CAP_NET_BIND_SERVICE present
3. **Kernel modules**: wireguard module loaded or built-in (5.6+)
4. **System config**: ip_forward v4/v6 enabled, nftables available
5. **Filesystem**: data directory exists, writable, correct permissions, disk space
6. **Database**: file accessible, schema version current, integrity check (`PRAGMA integrity_check`), table row counts, size
7. **Network**: default interface, public IP detection, port availability for all configured listen ports
8. **TLS**: certificate validity, expiry, domain match
9. **WireGuard state**: each interface up/down, peer counts, transfer totals, kernel state vs DB state comparison
10. **nftables state**: current rules summary, expected rules vs actual rules comparison
11. **Recent errors**: last 50 ERROR-level entries from journal (if journald accessible) or from internal ring buffer
12. **Reconciliation status**: any mismatches between DB and kernel state

Output format: plain text by default, `--json` flag for structured output that can be piped to Claude Code.

```bash
wgpilot diagnose --json > /tmp/diagnostic.json
```

---

## Internal Error Ring Buffer

The application keeps the last 500 error and warning log entries in an in-memory ring buffer, independent of journald. This ensures diagnostics are available even if journald is misconfigured or the user can't access it.

```go
package logging

import "sync"

type RingBuffer struct {
    mu      sync.RWMutex
    entries []LogEntry
    size    int
    pos     int
}

type LogEntry struct {
    Timestamp time.Time
    Level     slog.Level
    Message   string
    Attrs     map[string]any
}

func NewRingBuffer(size int) *RingBuffer {
    return &RingBuffer{
        entries: make([]LogEntry, size),
        size:    size,
    }
}

func (rb *RingBuffer) Write(entry LogEntry) {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    rb.entries[rb.pos%rb.size] = entry
    rb.pos++
}

func (rb *RingBuffer) Recent(n int) []LogEntry {
    rb.mu.RLock()
    defer rb.mu.RUnlock()
    // return last n entries in chronological order
    // ...
}
```

Exposed via:
- `GET /api/debug/logs?level=error&limit=100` (admin-only)
- `wgpilot diagnose` (reads from journal, falls back to ring buffer via API)

---

## Startup Logging

On every startup, log a complete environment snapshot at INFO level. This ensures that even in production, the log contains enough context to understand the environment when something goes wrong later.

```go
func logStartupInfo(logger *slog.Logger, cfg *Config) {
    logger.Info("wgpilot_starting",
        "version", version,
        "go_version", runtime.Version(),
        "os", runtime.GOOS,
        "arch", runtime.GOARCH,
        "pid", os.Getpid(),
        "uid", os.Getuid(),
        "gid", os.Getgid(),
        "data_dir", cfg.DataDir,
        "listen", cfg.Listen,
        "log_level", cfg.LogLevel,
        "dev_mode", cfg.DevMode,
        "tls_mode", cfg.TLS.Mode,
        "component", "main",
    )

    // Log capability check results
    caps := checkCapabilities()
    logger.Info("capabilities_check",
        "net_admin", caps.NetAdmin,
        "net_bind_service", caps.NetBindService,
        "component", "main",
    )

    // Log kernel/module state
    wgInfo := detectWireGuard()
    logger.Info("wireguard_detection",
        "module_loaded", wgInfo.ModuleLoaded,
        "kernel_builtin", wgInfo.KernelBuiltIn,
        "kernel_version", wgInfo.KernelVersion,
        "component", "wg",
    )

    // Log existing interfaces found
    ifaces := listWireguardInterfaces()
    logger.Info("existing_interfaces",
        "count", len(ifaces),
        "names", ifaces,
        "component", "wg",
    )

    // Log DB state
    logger.Info("database_state",
        "path", cfg.DataDir + "/data.db",
        "schema_version", db.SchemaVersion(),
        "component", "db",
    )
}
```

---

## Reconciliation Logging

The startup reconciliation (DB state vs kernel state) is a common source of subtle bugs. Log every mismatch found and every corrective action taken.

```go
func (m *Manager) Reconcile(logger *slog.Logger) error {
    logger.Info("reconciliation_start", "component", "reconcile")

    // Compare DB networks vs kernel interfaces
    dbNetworks, _ := m.store.ListNetworks()
    kernelDevices, _ := m.wg.Devices()

    for _, net := range dbNetworks {
        dev, found := findDevice(kernelDevices, net.Interface)
        if !found {
            logger.Warn("reconcile_missing_interface",
                "network_id", net.ID,
                "interface", net.Interface,
                "action", "recreating",
                "component", "reconcile",
            )
            m.CreateInterface(net) // this has its own detailed logging
            continue
        }

        // Compare peer counts
        dbPeers, _ := m.store.ListPeers(net.ID)
        if len(dbPeers) != len(dev.Peers) {
            logger.Warn("reconcile_peer_count_mismatch",
                "network_id", net.ID,
                "interface", net.Interface,
                "db_peers", len(dbPeers),
                "kernel_peers", len(dev.Peers),
                "action", "syncing",
                "component", "reconcile",
            )
        }

        // Compare each peer's config
        for _, dbPeer := range dbPeers {
            kernelPeer, found := findPeer(dev.Peers, dbPeer.PublicKey)
            if !found {
                logger.Warn("reconcile_missing_peer",
                    "peer_id", dbPeer.ID,
                    "peer_name", dbPeer.Name,
                    "network_id", net.ID,
                    "action", "adding_to_kernel",
                    "component", "reconcile",
                )
                continue
            }

            // Check allowed IPs match
            if !allowedIPsMatch(dbPeer.AllowedIPs, kernelPeer.AllowedIPs) {
                logger.Warn("reconcile_allowed_ips_mismatch",
                    "peer_id", dbPeer.ID,
                    "peer_name", dbPeer.Name,
                    "db_allowed_ips", dbPeer.AllowedIPs,
                    "kernel_allowed_ips", kernelPeer.AllowedIPs,
                    "action", "updating_kernel",
                    "component", "reconcile",
                )
            }

            // Check endpoint match
            if dbPeer.Endpoint != "" && !endpointMatches(dbPeer.Endpoint, kernelPeer.Endpoint) {
                logger.Debug("reconcile_endpoint_mismatch",
                    "peer_id", dbPeer.ID,
                    "db_endpoint", dbPeer.Endpoint,
                    "kernel_endpoint", kernelPeer.Endpoint,
                    "action", "no_action_dynamic",
                    "component", "reconcile",
                )
            }
        }
    }

    // Check for kernel interfaces not in DB (orphaned)
    for _, dev := range kernelDevices {
        if !hasNetwork(dbNetworks, dev.Name) {
            logger.Warn("reconcile_orphaned_interface",
                "interface", dev.Name,
                "peer_count", len(dev.Peers),
                "action", "ignored_not_managed",
                "component", "reconcile",
            )
        }
    }

    logger.Info("reconciliation_complete", "component", "reconcile")
    return nil
}
```

---

## Error Wrapping Convention

All errors must be wrapped with context using `fmt.Errorf` with `%w`. This creates a chain that Claude Code can trace from the API response back to the root cause.

```go
// In the DB layer:
return fmt.Errorf("db: get peer %d: %w", peerID, err)

// In the WG layer:
return fmt.Errorf("wg: add peer to %s: %w", ifaceName, err)

// In the HTTP handler:
return fmt.Errorf("handler: create peer for network %d: %w", networkID, err)
```

The API error response includes the full error chain in dev mode, sanitized message in production:

```go
func writeError(w http.ResponseWriter, r *http.Request, err error, status int, devMode bool) {
    resp := ErrorResponse{
        Error:     sanitizeError(err), // user-safe message
        Code:      errorCode(err),     // machine-readable code like "PEER_NOT_FOUND"
        RequestID: requestIDFromContext(r.Context()),
    }

    if devMode {
        resp.Detail = err.Error()           // full error chain
        resp.Stack = captureStack(3)        // abbreviated stack trace
        resp.Hint = classifyError(err)      // actionable suggestion
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(resp)
}
```

Dev mode error response example:

```json
{
    "error": "Failed to create peer",
    "code": "WG_PEER_ADD_FAILED",
    "request_id": "req_a1b2c3d4e5f6",
    "detail": "handler: create peer for network 1: wg: add peer to wg0: operation not permitted",
    "stack": "internal/server/peers.go:89 → internal/wg/manager.go:156 → netlink.ConfigureDevice",
    "hint": "missing CAP_NET_ADMIN capability — check systemd unit AmbientCapabilities"
}
```

---

## Structured Error Codes

Every error returned by the API has a machine-readable code. This helps Claude Code pattern-match on errors without parsing human-readable strings.

```go
const (
    // Network errors
    ErrNetworkNotFound       = "NETWORK_NOT_FOUND"
    ErrNetworkAlreadyExists  = "NETWORK_ALREADY_EXISTS"
    ErrInterfaceCreateFailed = "INTERFACE_CREATE_FAILED"
    ErrInterfaceUpFailed     = "INTERFACE_UP_FAILED"
    ErrSubnetConflict        = "SUBNET_CONFLICT"
    ErrPortInUse             = "PORT_IN_USE"

    // Peer errors
    ErrPeerNotFound          = "PEER_NOT_FOUND"
    ErrPeerAlreadyExists     = "PEER_ALREADY_EXISTS"
    ErrPeerAddFailed         = "WG_PEER_ADD_FAILED"
    ErrIPExhausted           = "IP_POOL_EXHAUSTED"
    ErrInvalidAllowedIPs     = "INVALID_ALLOWED_IPS"

    // Auth errors
    ErrUnauthorized          = "UNAUTHORIZED"
    ErrSessionExpired        = "SESSION_EXPIRED"
    ErrInvalidCredentials    = "INVALID_CREDENTIALS"

    // System errors
    ErrWGModuleNotLoaded     = "WG_MODULE_NOT_LOADED"
    ErrCapabilityMissing     = "CAPABILITY_MISSING"
    ErrNFTablesUnavailable   = "NFTABLES_UNAVAILABLE"
    ErrDatabaseCorrupted     = "DATABASE_CORRUPTED"

    // General
    ErrValidation            = "VALIDATION_ERROR"
    ErrInternal              = "INTERNAL_ERROR"
)
```

---

## Panic Recovery

Every goroutine and every HTTP request must be wrapped in panic recovery. Panics are logged as ERROR with full stack trace, then the goroutine or request is terminated gracefully — not the whole process.

```go
func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if rec := recover(); rec != nil {
                    stack := debug.Stack()
                    logger.Error("panic_recovered",
                        "panic", fmt.Sprintf("%v", rec),
                        "stack", string(stack),
                        "method", r.Method,
                        "path", r.URL.Path,
                        "request_id", requestIDFromContext(r.Context()),
                        "component", "http",
                    )
                    http.Error(w, `{"error":"internal error","code":"INTERNAL_ERROR"}`, 500)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## Configuration

```yaml
# config.yaml
log:
  level: info          # trace, debug, info, warn, error
  format: json         # json (production) or text (dev)

dev_mode: false        # enables: debug logging, request/response bodies,
                       # SQL query logging, diagnostic endpoints,
                       # verbose error responses with stack traces
```

CLI override:

```bash
wgpilot serve --log-level=debug --dev-mode    # max verbosity
wgpilot serve --log-level=trace               # even more than debug (future use)
wgpilot serve                                 # production defaults (info, json)
```

Environment variable override (highest priority):

```bash
WG_LOG_LEVEL=debug WG_DEV_MODE=true wgpilot serve
```

Priority: env vars > CLI flags > config.yaml > defaults.

---

## Implementation Checklist

When implementing logging across the codebase, every function that can fail must:

- [ ] Log the operation start at DEBUG level with all input parameters
- [ ] Log success at INFO or DEBUG level with relevant output/result
- [ ] Log failure at ERROR level with: error, error_type, operation, component, entity IDs, and a hint where applicable
- [ ] Wrap errors with `fmt.Errorf("context: %w", err)` — never return bare errors
- [ ] Include `request_id` or `task_id` from context in every log entry
- [ ] Never log passwords, private keys, session tokens, or preshared keys
- [ ] Log public keys (they're safe and needed for debugging peer issues)
