# Claude Code Instruction: Implement Logging, Debugging & Diagnostics

## Context

You are working on `wgpilot`, a WireGuard management tool written in Go with an embedded React SPA. The project uses `log/slog` from the Go standard library. The application manages WireGuard interfaces via `wgctrl-go` and `vishvananda/netlink`, uses SQLite for persistence, and runs as a systemd service.

This instruction adds a comprehensive two-tier logging and diagnostics system to the codebase. The goal: when a bug occurs in production or during development, Claude Code (or any developer) has immediate access to enough structured information to trace the issue from symptom to root cause without adding more logging after the fact.

## Spec File

Before writing any code, read the full logging specification:

```
docs/operations/logging-debugging.md
```

Place the attached spec content into that file path within the existing doc tree. This is the source of truth for everything below.

---

## Two Logging Modes

### Production (default)

- Log level: `INFO`
- Format: structured JSON to stdout (captured by journald)
- Content: operational events only — what happened, not how
- No request/response bodies, no SQL queries, no verbose internals

### Development (`--dev-mode` or `WG_LOG_LEVEL=debug`)

- Log level: `DEBUG`
- Format: human-readable text with source file:line on every entry
- Content: everything — full request/response bodies, SQL queries with parameters and timing, every WireGuard kernel call and response, nftables rule changes, goroutine/memory info, full error chains with stack traces and actionable hints

---

## Implementation Tasks

### Task 1 — Logging Package (`internal/logging/`)

Create the core logging infrastructure:

1. `internal/logging/logger.go` — logger factory that creates `*slog.Logger` based on config (level, dev mode, add source). Dev mode uses `slog.NewTextHandler`, production uses `slog.NewJSONHandler`. No global logger — inject via dependency injection everywhere.

2. `internal/logging/ring.go` — in-memory ring buffer (500 entries) that captures ERROR and WARN entries independently of journald. Thread-safe with `sync.RWMutex`. Exposes `Write(LogEntry)` and `Recent(n int) []LogEntry`. This is the fallback when journald is inaccessible.

3. `internal/logging/context.go` — helpers to store/retrieve `request_id` (format: `req_<12 hex chars>`) and `task_id` (format: `task_<name>_<unix timestamp>`) from `context.Context`. Every log entry in a request chain must carry the same `request_id`.

### Task 2 — HTTP Middleware (`internal/middleware/`)

1. `internal/middleware/request_id.go` — generates `request_id`, injects into context, adds to response header `X-Request-ID`.

2. `internal/middleware/request_logger.go` — logs every HTTP request at completion with: request_id, method, path, remote_addr, user_agent, status, duration_ms, bytes_written, authenticated user (if any). In dev mode: also log request body and response body (cap at 1MB). Log level: INFO for 2xx, WARN for 4xx, ERROR for 5xx.

3. `internal/middleware/recovery.go` — panic recovery that logs ERROR with full `debug.Stack()`, request context, and returns a JSON error with `request_id`. Never crashes the process.

### Task 3 — Database Wrapper (`internal/db/`)

Wrap all database operations with logging:

- Dev mode: log every query with SQL text, parameters, duration, row count, and `request_id` from context
- All modes: log slow queries (>100ms) at WARN level
- All modes: log errors at ERROR level with query text, error, error_type
- Log transaction start/commit/rollback with duration in dev mode
- Log schema migrations at INFO level with before/after version

### Task 4 — WireGuard Management Logging (`internal/wg/`)

Every function that touches the kernel must log:

- DEBUG: operation start with all input parameters
- DEBUG: each sub-step completion (link add, addr assign, key configure, link up)
- INFO: operation success with summary
- ERROR: failure with error, error_type, component="wg", entity IDs, and a **hint**

Implement `classifyNetlinkError(err error) string` that translates cryptic netlink errors into actionable hints:

| Error contains | Hint |
|---|---|
| `operation not permitted` | `missing CAP_NET_ADMIN — check systemd AmbientCapabilities` |
| `file exists` | `interface already exists — check 'ip link show'` |
| `no such device` | `wireguard kernel module not loaded — run 'modprobe wireguard'` |
| `address already in use` | `listen port bound — check 'ss -ulnp | grep <port>'` |
| `no buffer space available` | `too many interfaces — check 'ip link | wc -l'` |
| default | `unknown netlink error — check 'dmesg | tail -20'` |

### Task 5 — nftables Logging (`internal/nft/`)

- Log every rule addition/removal at DEBUG with full rule definition
- In dev mode: dump current ruleset after every change
- Log errors with component="nft" and hints

### Task 6 — Auth Logging (`internal/auth/`)

- INFO: login success, logout, session created
- WARN: login failed (with reason: invalid_password, user_not_found, account_disabled), token expired, too many attempts
- **NEVER log**: passwords, private keys, session tokens, preshared keys, JWT secrets
- **Safe to log**: usernames, public keys, remote_addr, token issued_at/expires_at

### Task 7 — Error Handling Convention

Enforce across the entire codebase:

1. **Error wrapping**: every error return uses `fmt.Errorf("context: %w", err)` to build a traceable chain. Never return bare errors.

2. **API error responses**: every JSON error includes `error` (user-safe), `code` (machine-readable constant), `request_id`. Dev mode adds: `detail` (full error chain), `stack` (abbreviated), `hint` (actionable suggestion).

3. **Error codes**: define all codes as constants in `internal/errors/codes.go`. Group by subsystem: NETWORK_*, PEER_*, AUTH_*, WG_*, DB_*, SYSTEM_*. See the spec for the full list.

### Task 8 — Startup Logging

On every process start, log at INFO level:

- version, go_version, os, arch, pid, uid, gid
- config summary (data_dir, listen addr, log_level, dev_mode, tls_mode)
- capability check results (CAP_NET_ADMIN, CAP_NET_BIND_SERVICE)
- WireGuard detection (module loaded, kernel built-in, kernel version)
- existing WireGuard interfaces found
- database state (path, schema version, integrity)

This runs in production too — it's the baseline context for every log investigation.

### Task 9 — Reconciliation Logging

The startup reconciliation (DB vs kernel state) must log:

- INFO: reconciliation start/complete
- WARN: every mismatch found (missing interface, peer count mismatch, AllowedIPs mismatch, orphaned interfaces) with: what was expected, what was found, what corrective action was taken
- DEBUG: endpoint mismatches (these are expected for roaming clients, no action needed)

### Task 10 — Diagnostic Endpoint

`GET /api/debug/info` — admin-only, dev mode only. Returns a JSON snapshot of:

- version, go_version, os, arch, kernel version, uptime
- config (sanitized — no secrets)
- database stats (path, size, WAL size, table row counts, schema version)
- WireGuard state (each interface: name, state, address, port, public key, peer counts)
- system state (ip_forward v4/v6, nftables loaded, capabilities, memory, goroutines, open files)
- network (public IP, default interface, default gateway)
- TLS (mode, cert expiry, domains)

`GET /api/debug/logs?level=error&limit=100` — admin-only, reads from ring buffer. Returns recent error/warn entries as JSON array.

### Task 11 — Diagnostic CLI

`wgpilot diagnose` — runs without the service. Performs all checks from the spec:

1. Binary/runtime info
2. Capability check
3. Kernel module detection
4. System config (ip_forward, nftables)
5. Filesystem (data dir exists, writable, permissions, disk space)
6. Database (accessible, schema version, integrity check, table row counts, size)
7. Network (default interface, public IP, port availability for all configured listen ports)
8. TLS (cert validity, expiry, domain match)
9. WireGuard state (each interface, peer counts, transfer totals, DB vs kernel comparison)
10. nftables (current rules, expected vs actual comparison)
11. Recent errors (last 50 from journal if accessible, otherwise from API ring buffer)

Output: human-readable by default with PASS/WARN/FAIL markers. `--json` flag for structured output.

### Task 12 — Configuration

Add to `config.yaml`:

```yaml
log:
  level: info          # trace, debug, info, warn, error
  format: json         # json | text
dev_mode: false
```

Priority chain: env vars (`WG_LOG_LEVEL`, `WG_DEV_MODE`) > CLI flags (`--log-level`, `--dev-mode`) > config.yaml > defaults.

---

## Logging Attribute Standards

Every log entry must use these consistent attribute names:

| Attribute | Used when | Example |
|---|---|---|
| `request_id` | HTTP request chain | `req_a1b2c3d4e5f6` |
| `task_id` | Background operations | `task_poll_1701234567` |
| `component` | Always | `http`, `db`, `wg`, `nft`, `auth`, `reconcile`, `main` |
| `operation` | On errors | `create_interface`, `add_peer`, `login` |
| `error` | On errors | the error string |
| `error_type` | On errors | `*net.OpError`, `*sqlite.Error` |
| `hint` | On errors (where available) | actionable fix suggestion |
| `duration_ms` | Timed operations | `42` |
| `network_id` | Network-related | `1` |
| `interface` | WG interface-related | `wg0` |
| `peer_id` | Peer-related | `5` |
| `peer_name` | Peer-related | `My Phone` |
| `user` | Auth-related | `admin` |
| `method` | HTTP | `POST` |
| `path` | HTTP | `/api/networks/1/peers` |
| `status` | HTTP | `201` |
| `query` | DB (dev mode) | `SELECT * FROM peers WHERE ...` |
| `args` | DB (dev mode) | `[1, "active"]` |

---

## Validation Checklist

After implementation, verify:

- [ ] Logger is injected via constructor/dependency injection everywhere — no `slog.Default()` outside `main()`
- [ ] Every function that can fail logs errors at ERROR level with: error, error_type, operation, component, relevant IDs
- [ ] Every error is wrapped with `fmt.Errorf("context: %w", err)` — no bare error returns
- [ ] Every HTTP response >= 400 includes JSON body with `error`, `code`, `request_id`
- [ ] Dev mode error responses additionally include `detail`, `stack`, `hint`
- [ ] `request_id` propagates through context from HTTP middleware to DB/WG/nft layers
- [ ] Passwords, private keys, session tokens, preshared keys are NEVER logged
- [ ] Public keys ARE logged (safe and needed for peer debugging)
- [ ] Slow DB queries (>100ms) are logged at WARN in all modes
- [ ] Panics are recovered in HTTP handlers and background goroutines, logged with full stack
- [ ] `wgpilot diagnose` runs without the service and covers all 12 check categories
- [ ] `wgpilot diagnose --json` outputs valid parseable JSON
- [ ] Ring buffer captures last 500 error/warn entries independently of journald
- [ ] Startup logging covers version, capabilities, WG detection, DB state at INFO level
- [ ] Reconciliation logs every mismatch with expected vs actual and corrective action
- [ ] `classifyNetlinkError` provides hints for all common netlink failure modes
- [ ] All error codes are defined as constants, not inline strings
