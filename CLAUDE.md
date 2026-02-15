# CLAUDE.md — wgpilot Project Intelligence

> Claude Code reads this file automatically at the start of every session.
> It defines the rules, conventions, and architecture constraints for the entire project.

## Project Overview

wgpilot is a WireGuard management tool. Single Go binary, embedded React SPA, SQLite database. Manages WireGuard interfaces via kernel netlink API (not CLI). Runs as a systemd service with CAP_NET_ADMIN.

## Architecture Rules (Never Violate)

1. **Single binary.** Everything ships as one Go binary with the frontend embedded via `go:embed`. No external runtime dependencies. No sidecar processes.

2. **No shell-outs.** Never call `wg`, `wg-quick`, `ip`, `iptables`, or any CLI tool via `exec.Command`. Use `wgctrl-go` for WireGuard, `vishvananda/netlink` for network interfaces, `google/nftables` for firewall rules. The only exception is `wgpilot diagnose` reading from `journalctl`.

3. **SQLite is the source of truth.** The database defines what the state should be. On startup, reconcile kernel state against the database. Never parse `/etc/wireguard/*.conf` files during normal operation (import is a one-time exception).

4. **No global state.** No global logger, no global DB connection, no global config. Everything is injected via constructors. `main()` is the only place that wires dependencies together.

5. **Context propagation.** Every function that does I/O takes `context.Context` as its first parameter. Request IDs flow through context from HTTP middleware to every subsystem.

6. **Errors are wrapped, not returned bare.** Every `return err` must be `return fmt.Errorf("context: %w", err)`. The error chain must be traceable from API response to root cause.

## Go Package Layout

```
cmd/wgpilot/          → CLI entrypoint only. Wires dependencies, starts server.
internal/server/    → HTTP handlers, routes, middleware. No business logic here.
internal/wg/        → WireGuard interface management (wgctrl + netlink). No HTTP awareness.
internal/nft/       → nftables rule management. No HTTP awareness.
internal/db/        → SQLite repository. All SQL lives here. No business logic.
internal/auth/      → JWT, sessions, password hashing. No HTTP awareness.
internal/config/    → Config loading (yaml + env + flags). Pure data, no side effects.
internal/logging/   → Logger factory, ring buffer, context helpers.
internal/errors/    → Error codes, error classification, error response helpers.
internal/debug/     → Diagnostic endpoint, diagnose CLI, system checks.
frontend/           → React SPA (Vite + shadcn/ui + TanStack Query).
```

**Rules:**
- `internal/server/` calls into `internal/wg/`, `internal/db/`, `internal/auth/` — never the reverse.
- `internal/wg/`, `internal/nft/`, `internal/db/` are independent of each other. They don't import each other.
- `internal/server/` is the orchestration layer that coordinates between subsystems.
- No package under `internal/` imports `cmd/`.
- No circular dependencies. If two packages need to share types, extract the types into a shared `internal/model/` package.

## Go Coding Conventions

### Naming

- Interfaces: verb-based (`PeerStore`, `InterfaceManager`, `RuleManager`), not noun-based
- Constructors: `New<Type>(deps) (*Type, error)` — always return error even if current impl can't fail (future-proofing)
- Methods: receiver name is first letter of type (`func (m *Manager)`, `func (s *Store)`)
- Files: lowercase, underscore-separated, match the primary type they contain (`peer_store.go`, `nat_rules.go`)
- Test files: `<file>_test.go` in the same package (white-box testing)

### Error Handling

```go
// CORRECT: wrapped with context
if err := m.wg.AddPeer(ctx, iface, peer); err != nil {
    return fmt.Errorf("add peer %d to %s: %w", peer.ID, iface, err)
}

// WRONG: bare error return
if err := m.wg.AddPeer(ctx, iface, peer); err != nil {
    return err
}

// WRONG: swallowed error
m.wg.AddPeer(ctx, iface, peer) // ignoring error
```

### Logging

- Every function that can fail: log at ERROR on failure, DEBUG on start/success
- Always include: `component`, `operation`, relevant entity IDs
- Never log: passwords, private keys, session tokens, preshared keys
- Safe to log: public keys, usernames, IPs, ports
- Use `slog.With()` to set per-handler attributes rather than repeating in every call

### Structs and Methods

- Use pointer receivers consistently (never mix value and pointer receivers on the same type)
- Struct fields: group by purpose with blank lines between groups
- Exported fields only on API request/response types and model types
- Internal state fields are unexported

### SQL

- All SQL lives in `internal/db/` — no SQL strings anywhere else
- Use parameterized queries always — no string concatenation
- Use `sqlc` or hand-written queries with `database/sql` — no ORM
- Migrations are numbered SQL files embedded via `go:embed`
- Every query has a descriptive function name: `GetPeerByID`, `ListPeersByNetworkID`, `UpdatePeerAllowedIPs`

### HTTP Handlers

```go
// Handler signature: method on a server struct, receives injected dependencies
func (s *Server) handleCreatePeer(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    // 2. Validate input
    // 3. Call business logic (wg, db)
    // 4. Return response
}
```

- Parse and validate before doing anything
- Return early on errors with `writeError(w, r, err, status, s.devMode)`
- No business logic in handlers — delegate to `internal/wg/` and `internal/db/`
- Response types are explicit structs, not `map[string]any`

## Frontend Conventions

### Component Pattern

```tsx
// File: components/peers/peer-table.tsx
// One component per file. File name matches component name in kebab-case.

import { usePeers } from '@/api/peers';

interface PeerTableProps {
  networkId: string;
}

export function PeerTable({ networkId }: PeerTableProps) {
  const { data, isLoading, error } = usePeers(networkId);
  // ...
}
```

- Functional components only — no class components
- Props interface defined in the same file, directly above the component
- Hooks for data fetching live in `src/api/` — components never call `fetch` directly
- shadcn/ui for all base components — don't build custom buttons, dialogs, tables, etc.
- Tailwind for styling — no CSS files, no styled-components, no CSS modules

### State Management

- Server state: TanStack Query (fetch, cache, invalidate)
- Local UI state: `useState` / `useReducer`
- No Redux, no Zustand, no global state management library
- Form state: `react-hook-form` with `zod` validation schemas

### API Layer

- All API calls go through `src/api/client.ts` which handles auth headers, 401 redirects, and error parsing
- Each entity has its own file (`src/api/networks.ts`, `src/api/peers.ts`) exporting TanStack Query hooks
- Mutations invalidate relevant queries on success
- Optimistic updates only where the UI latency is noticeable (peer enable/disable toggle)

## Testing Expectations

- **Backend unit tests**: every function in `internal/wg/`, `internal/db/`, `internal/auth/`, `internal/nft/` has tests
- **Backend integration tests**: API handlers tested with `httptest` against a real SQLite DB (in-memory)
- **Frontend tests**: skip for now — focus on backend correctness first
- **Test naming**: `Test<Function>_<Scenario>` (e.g., `TestCreatePeer_DuplicatePublicKey`)
- **Table-driven tests** for functions with multiple input/output combinations
- **Mock interfaces** for kernel-level operations (`wgctrl`, `netlink`) — test business logic without root

## Security Non-Negotiables

- All API endpoints except `/health` and `/api/auth/login` require a valid JWT
- JWT secret is generated on first run and stored in the database — never in config files
- Passwords are bcrypt-hashed with cost 12
- Input validation on every API endpoint before processing
- Rate limiting on `/api/auth/login` (5 attempts per minute per IP)
- Private keys generated server-side are stored encrypted in SQLite (AES-256-GCM with a key derived from the JWT secret)
- CORS restricted to same-origin (the SPA is served from the same binary)
- All responses include: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Strict-Transport-Security` (when TLS enabled)

## File Naming

- Go: `snake_case.go`
- TypeScript/React: `kebab-case.tsx` for components, `kebab-case.ts` for utilities
- SQL migrations: `001_initial_schema.sql`, `002_add_peer_enabled.sql`
- Docs: `kebab-case.md`

## Commit Convention

```
feat(wg): add peer management with AllowedIPs validation
fix(db): handle concurrent writes with WAL mode
docs(spec): add monitoring specification
test(auth): add JWT expiry edge cases
refactor(server): extract middleware into separate package
```

## What NOT to Do

- Don't add dependencies without justification. The Go binary must stay lean.
- Don't use `interface{}` or `any` when a concrete type is possible.
- Don't write TODO comments — either implement it or create a GitHub issue.
- Don't add features not in the spec. If something seems missing, flag it.
- Don't use `panic()` except in truly unrecoverable init-time errors.
- Don't use `init()` functions — explicit initialization in `main()`.
- Don't create utility packages like `utils/`, `helpers/`, `common/`. Put functions where they belong.
- Don't return `(interface{}, error)` — return concrete types.
