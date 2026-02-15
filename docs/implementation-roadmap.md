# Implementation Roadmap — Phase-by-Phase Claude Code Prompts

> This file contains the ordered build plan for wgpilot v1.0. Each phase is a self-contained Claude Code session with explicit inputs, outputs, and validation criteria.
>
> **Rule**: Execute phases in order. Each phase depends on the previous ones compiling and passing tests. Do not skip ahead.

---

## Phase 0 — Project Scaffold

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/architecture/project-structure.md and docs/architecture/tech-stack.md.

Initialize the wgpilot project:

1. Create the Go module: `github.com/solvia-ch/wgpilot`
2. Create the full directory structure from project-structure.md
3. Set up `cmd/wgpilot/main.go` with cobra CLI:
   - `serve` subcommand (placeholder)
   - `init` subcommand (placeholder)
   - `diagnose` subcommand (placeholder)
   - `version` subcommand (prints version from ldflags)
   - `backup` / `restore` subcommands (placeholder)
   - `config check` subcommand (placeholder)
   - `update` subcommand (placeholder)
   - Global flags: --config, --data-dir, --log-level, --dev-mode
4. Create `internal/config/config.go` with the full config struct and loading logic (YAML + env + flags with correct priority)
5. Create go.mod with initial dependencies: cobra, slog, sqlite driver (modernc.org/sqlite)
6. Create Makefile with targets: build, test, lint, dev, frontend-build, clean
7. Create .goreleaser.yaml for linux/amd64, linux/arm64, linux/arm7
8. Create .github/workflows/ci.yml and release.yml
9. Set up frontend/ with Vite + React + TypeScript + Tailwind + shadcn/ui scaffold (empty app shell, no pages yet)
10. Verify: `go build ./cmd/wgpilot` compiles, `go test ./...` passes, `cd frontend && npm run build` succeeds

Do not implement any business logic yet. This is skeleton only. Every subcommand should print "not implemented" except `version`.
```

### Validates: project compiles, tests pass, frontend builds, CI config exists

---

## Phase 1 — Logging & Database Foundation

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/operations/logging-debugging.md and docs/architecture/data-model.md.

Implement the logging and database foundation:

LOGGING (read the full logging spec carefully):
1. internal/logging/logger.go — logger factory (slog, JSON/text, dev mode, add source)
2. internal/logging/ring.go — in-memory ring buffer (500 entries, thread-safe)
3. internal/logging/context.go — request_id and task_id context helpers

DATABASE:
4. internal/db/db.go — SQLite connection with WAL mode, foreign keys, busy timeout
5. internal/db/migrate.go — migration runner using embedded SQL files
6. internal/db/migrations/001_initial_schema.sql — complete schema from data-model.md:
   - settings table
   - networks table
   - peers table
   - peer_snapshots table
   - network_bridges table
   All with correct types, constraints, indexes, foreign keys with CASCADE deletes
7. internal/db/settings.go — CRUD for settings table
8. internal/db/networks.go — CRUD for networks table
9. internal/db/peers.go — CRUD for peers table
10. internal/db/snapshots.go — insert/query/compact for peer_snapshots

Wrap all DB operations with the logging spec:
- Dev mode: log every query with SQL, params, duration, request_id
- All modes: slow query warning at >100ms
- All modes: errors at ERROR level with full context

Write tests:
- Migration applies cleanly to empty DB
- Migration is idempotent (running twice doesn't error)
- CRUD operations for networks and peers
- Foreign key cascade: deleting network deletes its peers
- Snapshot compaction logic

Verify: `go test ./internal/...` all pass, including with -race flag.
```

### Validates: logging works in both modes, DB schema matches spec, all CRUD tested

---

## Phase 2 — Auth System

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/auth.md and docs/security-spec.md.

Implement authentication:

1. internal/auth/password.go — bcrypt hash/verify with cost 12
2. internal/auth/jwt.go — JWT generation and validation (HS256, configurable TTL)
3. internal/auth/session.go — cookie-based session management (HttpOnly, Secure, SameSite=Strict)
4. internal/auth/rate_limit.go — per-IP rate limiter for login (5/min, in-memory token bucket)
5. internal/server/middleware/auth.go — JWT validation middleware that extracts user from cookie, injects into context
6. internal/server/middleware/security_headers.go — all headers from security-spec.md
7. internal/server/routes_auth.go — POST /api/auth/login, POST /api/auth/setup, POST /api/auth/logout
8. internal/errors/codes.go — all error code constants from the logging spec

Implement the first-run OTP flow:
- init CLI command generates OTP, bcrypt-hashes it, stores in settings table
- POST /api/auth/setup accepts OTP + new username/password, creates admin, deletes OTP
- After setup_complete=true, the setup endpoint returns 409

Test:
- Login with correct credentials → JWT in cookie
- Login with wrong password → 401, no cookie
- Expired JWT → 401 on protected endpoint
- Rate limiting → 429 after 5 failures
- OTP setup flow → admin created, OTP deleted, subsequent setup returns 409
- Security headers present on all responses

Verify: `go test ./internal/auth/... ./internal/server/...` all pass with -race.
```

### Validates: full auth flow works, rate limiting works, security headers present

---

## Phase 3 — HTTP Server & Middleware Stack

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/architecture/api-surface.md and docs/operations/logging-debugging.md.

Build the HTTP server with full middleware stack:

1. internal/server/server.go — server struct with dependency injection (db, wg manager, nft manager, logger, config)
2. internal/server/routes.go — register all routes from api-surface.md (handlers can return 501 for unimplemented)
3. internal/middleware/request_id.go — generate request_id, inject into context, add X-Request-ID header
4. internal/middleware/request_logger.go — full request/response logging per the logging spec
5. internal/middleware/recovery.go — panic recovery with stack trace logging
6. internal/middleware/max_body.go — 1MB request body limit

Middleware chain order:
  recovery → security_headers → request_id → request_logger → max_body → auth → handler

7. Wire the serve command in cmd/wgpilot/main.go:
   - Load config
   - Create logger
   - Open DB and run migrations
   - Create server with all middleware
   - Implement systemd notify (sd_notify READY)
   - Implement graceful shutdown on SIGTERM/SIGINT
   - Implement config reload on SIGHUP
   - Implement watchdog heartbeat

8. internal/server/response.go — helper functions:
   - writeJSON(w, status, data)
   - writeError(w, r, err, status, devMode) — with error codes, request_id, dev mode extras

Test:
- Request ID is generated and present in response header
- Request ID propagates to handler context
- Panic in handler returns 500 with request_id, doesn't crash server
- Body > 1MB returns 413
- Unauthenticated request to protected endpoint returns 401
- Server starts and signals systemd notify

Verify: `go test ./...` passes. `go run ./cmd/wgpilot serve --dev-mode` starts and serves on configured port.
```

### Validates: server starts, middleware chain works, graceful shutdown works

---

## Phase 4 — WireGuard Management Layer

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/network-management.md, docs/features/peer-management.md, and docs/architecture/data-model.md.

Implement the WireGuard management layer:

1. internal/wg/interfaces.go — define interfaces: WireGuardController, LinkManager (for testability)
2. internal/wg/manager.go — Manager struct implementing network/peer lifecycle:
   - CreateInterface(ctx, network) error
   - DeleteInterface(ctx, name) error
   - AddPeer(ctx, iface, peer) error
   - RemovePeer(ctx, iface, publicKey) error
   - UpdatePeer(ctx, iface, peer) error
   - PeerStatus(iface) ([]PeerStatus, error)
   - Reconcile(ctx) error
3. internal/wg/ip_alloc.go — IP pool allocator:
   - Given a subnet, track allocated IPs
   - .1 is always reserved for server
   - Allocate returns next available, error on exhaustion
   - Release returns IP to pool
4. internal/wg/config_gen.go — generate peer .conf files from templates
5. internal/wg/qr.go — generate QR code image from .conf content
6. internal/wg/classify.go — classifyNetlinkError with all hints from the logging spec

Every function must follow the logging spec:
- DEBUG: operation start with all parameters
- DEBUG: each sub-step completion
- INFO: operation success summary
- ERROR: failure with error, error_type, component, hint

7. internal/wg/reconcile.go — startup reconciliation: compare DB vs kernel, log every mismatch, correct state

8. internal/testutil/mock_wg.go — mock implementations of WireGuardController and LinkManager

Test (using mocks — no real kernel interaction in tests):
- CreateInterface calls LinkAdd, AddrAdd, ConfigureDevice, LinkSetUp in correct order
- DeleteInterface removes peers first, then link
- IP allocation: sequential, skips .1, detects exhaustion
- Config generation: produces valid WireGuard config syntax
- Reconciliation: detects missing interfaces, missing peers, config mismatches
- classifyNetlinkError returns correct hints for all known error patterns

Verify: `go test ./internal/wg/...` all pass with -race.
```

### Validates: WG management works against mocks, IP allocation is correct, config generation is valid

---

## Phase 5 — nftables Management

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/network-management.md and docs/features/multi-network.md.

Implement nftables rule management:

1. internal/nft/manager.go — NFTManager struct:
   - AddNATMasquerade(iface, subnet) error
   - RemoveNATMasquerade(iface) error
   - EnableInterPeerForwarding(iface) error
   - DisableInterPeerForwarding(iface) error
   - AddNetworkBridge(ifaceA, ifaceB, direction) error
   - RemoveNetworkBridge(ifaceA, ifaceB) error
   - DumpRules() (string, error)

All functions log per the logging spec. Dev mode dumps full ruleset after every change.

2. internal/nft/interfaces.go — define NFTableManager interface (for mocking)
3. internal/testutil/mock_nft.go — mock implementation tracking applied rules in memory

Test:
- NAT rules are added/removed correctly
- Inter-peer forwarding rules match topology mode
- Bridge rules are directional (a→b, b→a, bidirectional)
- Duplicate rule addition is idempotent

Verify: `go test ./internal/nft/...` passes.
```

### Validates: nftables management works against mocks

---

## Phase 6 — API Handlers (Network & Peer CRUD)

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/architecture/api-surface.md, docs/features/network-management.md, docs/features/peer-management.md, and docs/security-spec.md.

Implement all network and peer API handlers:

NETWORK HANDLERS (internal/server/routes_networks.go):
- POST   /api/networks — create network, validate input, create interface, add nftables rules
- GET    /api/networks — list all networks with peer counts and status
- GET    /api/networks/:id — get network detail with full config
- PUT    /api/networks/:id — update network settings (name, DNS, NAT toggle, inter-peer toggle)
- DELETE /api/networks/:id — delete network, remove all peers, remove interface, clean up rules

PEER HANDLERS (internal/server/routes_peers.go):
- POST   /api/networks/:id/peers — create peer, allocate IP, generate keys, add to WG
- GET    /api/networks/:id/peers — list peers with online status
- GET    /api/networks/:id/peers/:pid — get peer detail
- PUT    /api/networks/:id/peers/:pid — update peer (name, enabled, keepalive, endpoint)
- DELETE /api/networks/:id/peers/:pid — remove peer, release IP, remove from WG
- GET    /api/networks/:id/peers/:pid/config — download .conf file
- GET    /api/networks/:id/peers/:pid/qr — QR code image (PNG)

Every handler must:
- Validate all input per security-spec.md before processing
- Return structured errors with error codes
- Log per the logging spec
- Handle topology mode differences (gateway vs site-to-site vs hub-routed) for AllowedIPs calculation

Integration tests (real SQLite, mocked WG and nftables):
- Full CRUD lifecycle: create network → add peers → update peer → delete peer → delete network
- Validation errors return 400 with field-level details
- Subnet conflict detection
- Port conflict detection
- IP exhaustion returns appropriate error
- Topology modes produce different AllowedIPs
- Config download returns valid WireGuard config
- QR endpoint returns PNG image
- Cascading delete: network deletion removes all peers

Verify: `go test ./internal/server/...` all pass with -race.
```

### Validates: full API works end-to-end with mocked kernel, all error cases handled

---

## Phase 7 — Monitoring & Live Status

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/monitoring.md and docs/operations/logging-debugging.md.

Implement monitoring:

1. internal/server/routes_status.go:
   - GET /api/status — live interface stats from kernel (peer status, transfer, handshakes)
   - GET /api/networks/:id/events — SSE endpoint pushing peer status updates every 5 seconds

2. internal/server/routes_debug.go:
   - GET /api/debug/info — full diagnostic JSON snapshot (admin-only, dev mode only)
   - GET /api/debug/logs — recent errors from ring buffer (admin-only)
   - GET /health — public health check

3. Background polling goroutine:
   - Polls WG peer status every 30 seconds
   - Stores snapshots in peer_snapshots table
   - Detects online/offline transitions, logs at INFO
   - Pushes changes to SSE subscribers

4. Snapshot compaction:
   - Runs hourly
   - 5-min granularity for last 24h, hourly for 30d, daily for 1y

5. Prometheus metrics endpoint: GET /metrics
   - wg_peers_total, wg_peers_online (per network)
   - wg_transfer_bytes_total (per network, per direction)
   - wg_peer_last_handshake_seconds (per peer)
   - wg_interface_up (per network)
   - wg_webui_http_requests_total (by method, status)

6. Implement `wgpilot diagnose` CLI:
   - All 12 check categories from the logging spec
   - Plain text output with PASS/WARN/FAIL markers
   - --json flag for structured output
   - Runs without the service (reads DB and kernel state directly)

Test:
- Status endpoint returns correct peer data
- SSE connection receives updates
- Health endpoint returns correct structure
- Snapshot compaction reduces row count correctly
- Diagnose CLI produces valid output and valid JSON with --json

Verify: `go test ./...` passes.
```

### Validates: monitoring works, SSE works, diagnose CLI works

---

## Phase 8 — Frontend (Dashboard & CRUD)

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/network-management.md, docs/features/peer-management.md, docs/features/monitoring.md, and docs/architecture/api-surface.md.

Build the React frontend:

1. src/api/client.ts — fetch wrapper with auth, error handling, 401 redirect
2. src/api/networks.ts — TanStack Query hooks for network CRUD
3. src/api/peers.ts — TanStack Query hooks for peer CRUD
4. src/api/status.ts — status polling hook
5. src/hooks/use-sse.ts — SSE hook for live peer status updates
6. src/hooks/use-auth.ts — auth state management

7. Layout:
   - src/components/layout/app-shell.tsx — sidebar + header + content
   - src/components/layout/nav.tsx — network list in sidebar

8. Dashboard:
   - src/components/dashboard/stats-cards.tsx — network count, peer count, transfer totals
   - src/components/dashboard/peer-status-list.tsx — all peers with live online/offline indicators
   - src/components/dashboard/transfer-chart.tsx — Recharts line chart for transfer history

9. Network management:
   - src/components/networks/network-list.tsx — card per network
   - src/components/networks/network-form.tsx — create/edit network dialog (mode selector, subnet, port, toggles)

10. Peer management:
    - src/components/peers/peer-table.tsx — table with status, last seen, transfer, actions
    - src/components/peers/peer-form.tsx — create/edit peer dialog
    - src/components/peers/peer-config-modal.tsx — shows QR code + config + download button

Use shadcn/ui for all base components. Tailwind for styling. No custom CSS.
Responsive: works on desktop and tablet. Mobile is nice-to-have.

Verify: `npm run build` succeeds with no TypeScript errors. Full Go binary builds with embedded frontend.
```

### Validates: frontend builds, all pages render, API integration works

---

## Phase 9 — Setup Wizard

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/first-run.md and docs/features/auth.md.

Implement the setup wizard:

1. src/components/setup/wizard.tsx — multi-step wizard container with progress indicator
2. src/components/setup/step-admin.tsx — create admin account (using OTP from install)
3. src/components/setup/step-server.tsx — server identity (auto-detect public IP, optional hostname, DNS)
4. src/components/setup/step-network.tsx — first network (name, mode, subnet, port, NAT toggle)
5. src/components/setup/step-peer.tsx — first peer (name, role, tunnel type) → shows QR + config on completion

Backend:
6. Add setup_complete check middleware: if setup_complete=false, all non-setup routes redirect to /setup
7. GET /api/setup/status — returns { complete: bool, current_step: int }
8. POST /api/setup/step/:n — process each wizard step
9. Auto-detect public IP via multiple fallback services (ipify, icanhazip, ifconfig.me)

Edge cases:
- Browser closed mid-wizard → resume at correct step on next visit
- Existing WireGuard interfaces detected → offer import option
- Port 443 taken → fall back to 8443, inform user
- ACME fails → fall back to self-signed, inform user

Test:
- Full wizard flow: OTP → admin → server → network → peer → setup_complete=true
- Resume after interruption at each step
- Setup endpoint returns 409 after completion
- Import detection finds existing wg interfaces

Verify: full wizard flow works in browser against running backend.
```

### Validates: wizard flow works end-to-end, edge cases handled

---

## Phase 10 — Install Script & TLS

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/install-script.md, docs/operations/tls.md, and docs/operations/service.md.

1. install.sh — complete install script:
   - Root check
   - OS detection (Ubuntu, Debian, Fedora, CentOS, Rocky, Alma)
   - Architecture detection (amd64, arm64, arm7)
   - WireGuard kernel module check and install
   - IP forwarding enable (sysctl, persistent)
   - Download binary from GitHub releases (latest version via API)
   - Create wg-webui system user
   - Create /var/lib/wgpilot, /etc/wgpilot with correct ownership/permissions
   - Generate default config.yaml
   - Install systemd unit (hardened, from service.md)
   - Run `wgpilot init` to generate OTP
   - Start and enable service
   - Print URL + OTP

2. internal/tls/tls.go — TLS manager:
   - Self-signed mode: generate self-signed cert on first run, store in data_dir/certs/
   - ACME mode: use golang.org/x/crypto/acme/autocert with file-based cache in data_dir/certs/
   - Manual mode: load cert/key from config paths
   - Detect mode from config, fall back gracefully (ACME fails → self-signed)

3. Self-update mechanism:
   - `wgpilot update` — check GitHub releases API, download new binary, replace self, signal systemd restart
   - `wgpilot update --check` — just print available version without installing

Test:
- install.sh passes shellcheck with no errors
- TLS self-signed generates valid certificate
- TLS ACME mode configures autocert correctly (mock test)
- Self-update detects newer version correctly

Verify: `shellcheck install.sh` passes. TLS tests pass.
```

### Validates: install script is correct, TLS works in all modes

---

## Phase 11 — Multi-Network & Bridging

### Prompt for Claude Code:

```
Read CLAUDE.md, then read docs/features/multi-network.md and docs/features/network-management.md.

Implement multi-network support:

1. Multiple WG interfaces (wg0, wg1, wg2, ...) — auto-assigned on network creation
2. Each network is fully independent: own subnet, own port, own keypair
3. Network isolation by default (no traffic between wg0 and wg1)

4. Network bridging:
   - POST /api/bridges — create bridge between two networks
   - DELETE /api/bridges/:id — remove bridge
   - Bridge has direction: a→b, b→a, bidirectional
   - Bridge creates nftables FORWARD rules between interfaces
   - Optional CIDR filtering on bridges

5. Update dashboard to show multiple networks
6. Update nav to list networks in sidebar

Test:
- Create two networks with different subnets and ports
- Networks are isolated (no forwarding rules between them)
- Bridge creation adds correct forwarding rules
- Bridge deletion removes rules
- Cascading: network deletion removes associated bridges
- Subnet conflict detection across networks

Verify: `go test ./...` passes. Two networks can be created via API.
```

### Validates: multi-network works, bridging works, isolation is default

---

## Phase 12 — Polish, Hardening, Documentation

### Prompt for Claude Code:

```
Read CLAUDE.md and all docs.

Final hardening pass:

1. Review every error path: is the error wrapped? Is it logged? Does the API return the right status code and error code?

2. Review every handler: is input validated before processing? Are there any missing validation rules from security-spec.md?

3. Review logging: does every function that can fail follow the logging spec? Are request_ids propagating correctly?

4. Add missing tests identified during review.

5. Run `go vet ./...` and fix any issues.
6. Run `staticcheck ./...` and fix any issues (install if needed).
7. Run `go test -race ./...` and fix any races.
8. Run `npm run lint` in frontend and fix any issues.

9. Write README.md:
   - One-liner install command
   - Screenshot placeholder
   - Feature list
   - Quick start guide
   - Configuration reference
   - CLI reference
   - Building from source
   - License

10. Verify full flow: install → setup wizard → create network → add peer → download config → dashboard shows status

Tag as v0.1.0.
```

### Validates: everything works end-to-end, no lint errors, no race conditions

---

## Execution Notes

- **One phase per Claude Code session.** Start fresh for each phase — load only the docs listed in the prompt.
- **Always prefix with "Read CLAUDE.md"** — this ensures conventions are loaded.
- **Verify before proceeding.** Each phase has a verification step. Don't start the next phase until it passes.
- **If a phase is too large**, split it into sub-sessions. Phase 8 (frontend) is the most likely candidate.
- **Commit after each phase.** Use the commit convention from CLAUDE.md.
