# wgpilot — Specification Overview

**wgpilot** is a self-hosted WireGuard management appliance that ships as a single Go binary with an embedded React web UI, SQLite database, and programmatic WireGuard control via native Go libraries.

The binary embeds all frontend assets via `go:embed`, manages WireGuard interfaces through `wgctrl-go` + `netlink` + `nftables` (no shell-outs), and uses SQLite as the single source of truth. On startup, kernel state is reconciled from the database. Users install with a one-liner curl command, walk through a 4-step setup wizard, and manage VPN networks entirely through the browser.

**Design principles:** Single binary. No shell-outs. SQLite is the source of truth. Appliance model (install, configure, forget). Secure by default (unprivileged user, minimal Linux capabilities).

---

## Document Map

| File | Purpose | Related Docs |
|---|---|---|
| [architecture/tech-stack.md](architecture/tech-stack.md) | Go + React library choices and rationale | project-structure.md |
| [architecture/data-model.md](architecture/data-model.md) | SQLite schema, all tables, entity relationships | api-surface.md, network-management.md, peer-management.md |
| [architecture/api-surface.md](architecture/api-surface.md) | REST endpoints, request/response shapes, conventions | data-model.md, auth.md |
| [architecture/project-structure.md](architecture/project-structure.md) | Directory layout, Go packages, frontend structure, build pipeline, dev workflow | tech-stack.md |
| [features/install-script.md](features/install-script.md) | One-liner install, OS detection, prerequisites, binary download | service.md, updates.md |
| [features/first-run.md](features/first-run.md) | Setup wizard flow, 4 steps, edge cases, WG import | auth.md, network-management.md, peer-management.md |
| [features/network-management.md](features/network-management.md) | Network CRUD, topology modes, WG interface lifecycle, nftables rules, IP allocation | data-model.md, multi-network.md, peer-management.md |
| [features/peer-management.md](features/peer-management.md) | Peer CRUD, key generation, config generation, QR codes, AllowedIPs logic | data-model.md, network-management.md |
| [features/monitoring.md](features/monitoring.md) | Dashboard, live stats, SSE, Prometheus metrics, historical snapshots, alerts, logging | data-model.md, api-surface.md |
| [features/auth.md](features/auth.md) | JWT authentication, bcrypt, sessions, login flow, first-run token, security hardening | api-surface.md, first-run.md |
| [features/multi-network.md](features/multi-network.md) | Multiple WG interfaces, isolation, bridging, subnet validation | network-management.md, data-model.md |
| [operations/service.md](operations/service.md) | systemd unit, capabilities, filesystem layout, CLI subcommands, config file, signals | install-script.md, tls.md |
| [operations/tls.md](operations/tls.md) | HTTPS modes (self-signed, ACME, manual), cert management | service.md, first-run.md |
| [operations/updates.md](operations/updates.md) | Self-update mechanism, GoReleaser, versioning, roadmap | install-script.md, project-structure.md |
| [decisions/adr-001-no-mesh.md](decisions/adr-001-no-mesh.md) | Why mesh topology was excluded, full decisions log | network-management.md |

---

## Build Phases

| Phase | Task | Spec Files |
|---|---|---|
| 1 | SQLite schema + repository layer | [data-model.md](architecture/data-model.md) |
| 2 | CLI, systemd, config loading | [service.md](operations/service.md), [project-structure.md](architecture/project-structure.md) |
| 3 | JWT auth + admin setup | [auth.md](features/auth.md) |
| 4 | Network CRUD + WG interface mgmt | [network-management.md](features/network-management.md), [data-model.md](architecture/data-model.md) |
| 5 | Peer CRUD + config generation + QR | [peer-management.md](features/peer-management.md), [data-model.md](architecture/data-model.md) |
| 6 | Dashboard API + SSE + metrics | [monitoring.md](features/monitoring.md) |
| 7 | Setup wizard (frontend) | [first-run.md](features/first-run.md), [auth.md](features/auth.md) |
| 8 | Install script | [install-script.md](features/install-script.md) |
| 9 | TLS + ACME | [tls.md](operations/tls.md) |
| 10 | Multi-network + bridging | [multi-network.md](features/multi-network.md), [network-management.md](features/network-management.md) |
| 11 | Self-update mechanism | [updates.md](operations/updates.md) |
