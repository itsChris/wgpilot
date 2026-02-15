# Project Structure

> **Purpose**: Defines the directory layout, Go package organization, frontend structure, build pipeline, CI/CD configuration, and development workflow.
>
> **Related docs**: [tech-stack.md](tech-stack.md), [../operations/updates.md](../operations/updates.md)
>
> **Implements**: Root directory layout, `Makefile`, `.github/workflows/`, `.goreleaser.yaml`, `frontend/vite.config.ts`

---

## Repository Layout

```
github.com/itsChris/wgpilot/
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── .goreleaser.yaml
├── cmd/
│   └── wgpilot/
│       └── main.go                 # cobra CLI entrypoint
├── internal/
│   ├── server/
│   │   ├── server.go               # HTTP server setup, TLS, shutdown
│   │   ├── router.go               # route registration
│   │   ├── middleware.go            # auth, logging, recovery
│   │   └── handlers/
│   │       ├── auth.go
│   │       ├── setup.go
│   │       ├── networks.go
│   │       ├── peers.go
│   │       ├── bridges.go
│   │       ├── status.go
│   │       ├── settings.go
│   │       ├── alerts.go
│   │       └── system.go
│   ├── wg/
│   │   ├── manager.go
│   │   ├── device.go
│   │   ├── iface.go
│   │   ├── route.go
│   │   ├── keys.go
│   │   └── reconcile.go
│   ├── nft/
│   │   ├── manager.go
│   │   └── rules.go
│   ├── db/
│   │   ├── db.go
│   │   ├── networks.go
│   │   ├── peers.go
│   │   ├── bridges.go
│   │   ├── settings.go
│   │   ├── users.go
│   │   ├── snapshots.go
│   │   ├── audit.go
│   │   ├── alerts.go
│   │   └── migrations/
│   │       └── 001_initial.sql
│   ├── auth/
│   │   ├── jwt.go
│   │   ├── password.go
│   │   └── session.go
│   ├── config/
│   │   ├── config.go
│   │   └── defaults.go
│   ├── monitor/
│   │   ├── poller.go
│   │   ├── snapshots.go
│   │   ├── alerts.go
│   │   └── metrics.go
│   ├── tls/
│   │   ├── acme.go
│   │   ├── selfsigned.go
│   │   └── manager.go
│   └── updater/
│       └── updater.go
├── frontend/
│   ├── src/
│   │   ├── main.tsx
│   │   ├── api/
│   │   │   ├── client.ts
│   │   │   ├── networks.ts
│   │   │   ├── peers.ts
│   │   │   └── status.ts
│   │   ├── components/
│   │   │   ├── ui/                  # shadcn components
│   │   │   ├── layout/
│   │   │   │   ├── app-shell.tsx
│   │   │   │   └── nav.tsx
│   │   │   ├── networks/
│   │   │   │   ├── network-card.tsx
│   │   │   │   ├── network-form.tsx
│   │   │   │   └── network-list.tsx
│   │   │   ├── peers/
│   │   │   │   ├── peer-table.tsx
│   │   │   │   ├── peer-form.tsx
│   │   │   │   ├── peer-config-modal.tsx
│   │   │   │   └── peer-qr.tsx
│   │   │   ├── dashboard/
│   │   │   │   ├── stats-cards.tsx
│   │   │   │   ├── transfer-chart.tsx
│   │   │   │   └── peer-status-list.tsx
│   │   │   └── setup/
│   │   │       ├── wizard.tsx
│   │   │       ├── step-admin.tsx
│   │   │       ├── step-server.tsx
│   │   │       ├── step-network.tsx
│   │   │       └── step-peer.tsx
│   │   ├── hooks/
│   │   │   ├── use-sse.ts
│   │   │   └── use-auth.ts
│   │   ├── lib/
│   │   │   └── utils.ts
│   │   └── types/
│   │       └── api.ts
│   ├── index.html
│   ├── tailwind.config.js
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── package.json
├── install.sh
├── go.mod
├── go.sum
├── Makefile
├── LICENSE                          # MIT
├── README.md
└── docs/                            # specification documents
```

## Go Backend Package Descriptions

| Package | Purpose |
|---|---|
| `cmd/wgpilot` | CLI entrypoint (cobra), subcommand registration |
| `internal/server` | HTTP server, router, middleware (auth, logging, recovery) |
| `internal/server/handlers` | One handler file per resource (auth, setup, networks, peers, etc.) |
| `internal/wg` | WireGuard management: wgctrl device ops, netlink interface/route ops, key generation, startup reconciliation |
| `internal/nft` | nftables rule management: NAT masquerade, forwarding rules |
| `internal/db` | SQLite repository layer: connection, migrations, CRUD for all entities |
| `internal/auth` | JWT creation/validation, bcrypt password hashing, session management |
| `internal/config` | Config struct, loading from YAML/env/flags, defaults |
| `internal/monitor` | Periodic peer polling, snapshot writes/compaction, alert evaluation, Prometheus metrics |
| `internal/tls` | TLS certificate management: ACME, self-signed, manual |
| `internal/updater` | Self-update: GitHub releases check, binary replacement |

## Frontend Structure

See [tech-stack.md](tech-stack.md) for the full list of frontend libraries. The frontend is a React SPA built with Vite. Components are organized by feature domain (networks, peers, dashboard, setup) plus shared UI components from shadcn/ui.

The frontend embeds into the Go binary:

```go
//go:embed frontend/dist/*
var frontendFS embed.FS

func setupRoutes(mux *http.ServeMux) {
    // API routes
    mux.Handle("/api/", apiHandler())

    // SPA: serve frontend, fall back to index.html for client-side routing
    dist, _ := fs.Sub(frontendFS, "frontend/dist")
    fileServer := http.FileServer(http.FS(dist))

    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Try to serve the file directly
        path := r.URL.Path
        if f, err := dist.Open(strings.TrimPrefix(path, "/")); err == nil {
            f.Close()
            fileServer.ServeHTTP(w, r)
            return
        }
        // Fall back to index.html for SPA routing
        r.URL.Path = "/"
        fileServer.ServeHTTP(w, r)
    })
}
```

## Startup Sequence

```
main()
  ├── Parse CLI flags (cobra)
  ├── Load config.yaml
  ├── Open SQLite database
  │   ├── Enable WAL mode
  │   └── Run migrations (goose)
  ├── Initialize WG Manager
  │   └── Reconcile kernel state from DB
  │       ├── For each enabled network in DB:
  │       │   ├── Create interface if missing
  │       │   ├── Configure device (key, port)
  │       │   ├── Set IP address
  │       │   ├── Add all enabled peers
  │       │   ├── Apply nftables rules
  │       │   └── Bring interface up
  │       └── Remove orphaned interfaces not in DB
  ├── Initialize TLS (acme/self-signed/manual)
  ├── Start monitoring poller (30s interval)
  ├── Start snapshot compaction job (daily)
  ├── Start HTTP server
  ├── sd_notify(READY)
  ├── Start watchdog heartbeat (15s)
  └── Await shutdown signal
       ├── SIGTERM/SIGINT → graceful shutdown
       │   ├── Stop accepting connections
       │   ├── Finish in-flight requests (10s timeout)
       │   ├── Stop poller
       │   ├── Close database
       │   └── Exit 0
       └── SIGHUP → reload config
           ├── Re-read config.yaml
           ├── Update log level
           └── Refresh TLS certificates
```

## Build Pipeline

### Local Build (Makefile)

```makefile
.PHONY: build frontend-build go-build dev dev-api clean

build: frontend-build go-build

frontend-build:
	cd frontend && npm ci && npm run build

go-build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o wgpilot ./cmd/wgpilot

dev:
	cd frontend && VITE_API_PROXY=http://localhost:8080 npm run dev

dev-api:
	go run ./cmd/wgpilot serve --dev-mode --listen=0.0.0.0:8080

clean:
	rm -rf wgpilot frontend/dist
```

See [../operations/updates.md](../operations/updates.md) for GoReleaser config, GitHub Actions CI/release workflows, and versioning policy.

## Development Workflow

### Prerequisites

- Go 1.23+
- Node.js 20+
- Linux (for WireGuard kernel access) or develop API-only on macOS/Windows

### Dev Mode

Two terminals:

```bash
# Terminal 1: Vite dev server with HMR
make dev

# Terminal 2: Go backend (serves API on :8080, no embedded frontend)
make dev-api
```

Vite proxies `/api/*` to the Go backend. Frontend hot-reloads instantly. Backend restarts manually or via `air`/`watchexec`.

```typescript
// vite.config.ts
export default defineConfig({
    plugins: [react()],
    server: {
        proxy: {
            '/api': process.env.VITE_API_PROXY ?? 'http://localhost:8080',
        },
    },
});
```

### Testing

- **Go:** `go test ./...` — unit tests for all packages. Integration tests for WG manager require Linux + root (run in CI with elevated permissions).
- **Frontend:** `npm test` — component tests with Vitest + React Testing Library.
- **E2E:** Future consideration — Playwright against a running instance.

### Code Quality

- **Go:** `go vet`, `staticcheck`, `golangci-lint`
- **Frontend:** ESLint, Prettier, TypeScript strict mode
