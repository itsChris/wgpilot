# Technology Stack

> **Purpose**: Defines all libraries, frameworks, and infrastructure tools used in the wgpilot project with rationale for each choice.
>
> **Related docs**: [project-structure.md](project-structure.md)
>
> **Implements**: `go.mod`, `frontend/package.json`, `.goreleaser.yaml`, `.github/workflows/`

---

## Backend (Go)

| Component | Library | Purpose |
|---|---|---|
| HTTP server | `net/http` (stdlib) | Routing, middleware, TLS |
| CLI framework | `github.com/spf13/cobra` | Subcommands (serve, init, update, etc.) |
| WireGuard control | `golang.zx2c4.com/wireguard/wgctrl` | Peer/device management via netlink |
| Network interfaces | `github.com/vishvananda/netlink` | Create/delete interfaces, IPs, routes |
| Firewall | `github.com/google/nftables` | NAT, forwarding rules |
| Database | `modernc.org/sqlite` | Pure Go SQLite (no CGO) |
| Migrations | `github.com/pressly/goose/v3` | Schema migrations |
| Auth | `golang.org/x/crypto/bcrypt` | Password hashing |
| JWT | `github.com/golang-jwt/jwt/v5` | Session tokens |
| TLS | `golang.org/x/crypto/acme/autocert` | Auto Let's Encrypt |
| Logging | `log/slog` (stdlib) | Structured JSON logging |
| Systemd notify | `github.com/coreos/go-systemd/v22` | sd_notify integration |
| Config | `github.com/knadh/koanf/v2` | Config file + env + flags |
| QR codes | `github.com/skip2/go-qrcode` | QR code generation for peer configs |
| Embed | `embed` (stdlib) | Embed frontend assets in binary |

## Frontend (React)

| Component | Library | Purpose |
|---|---|---|
| Framework | React 18 | UI framework |
| Components | shadcn/ui (Radix primitives) | Accessible, styled components |
| Styling | Tailwind CSS | Utility-first CSS |
| Data fetching | TanStack Query | API cache, polling, mutations |
| Routing | TanStack Router | Type-safe client-side routing |
| Forms | react-hook-form + zod | Form handling + schema validation |
| Charts | Recharts | Transfer history graphs |
| QR display | qrcode.react | QR code rendering in browser |
| Build | Vite | Fast builds, HMR in dev |
| Language | TypeScript | Type safety |

## Infrastructure

| Component | Tool | Purpose |
|---|---|---|
| Build | GoReleaser | Cross-compile + GitHub Releases |
| CI | GitHub Actions | Test, lint, release |
| Service | systemd | Process management |
| TLS (optional) | Caddy | Reverse proxy if user prefers external TLS |
