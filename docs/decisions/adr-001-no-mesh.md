# ADR-001: No Mesh Topology

> **Purpose**: Records the decision to exclude mesh networking and documents all key architectural decisions with rationale.
>
> **Related docs**: [../features/network-management.md](../features/network-management.md)
>
> **Implements**: N/A (architectural constraint)

---

## Status

Accepted

## Context

WireGuard supports arbitrary peer-to-peer topologies. We considered supporting full mesh networking where every peer connects directly to every other peer, requiring no central hub.

## Decision

Mesh topology is excluded from wgpilot. Only three modes are supported:

1. **VPN Gateway** — hub-and-spoke with NAT to internet
2. **Site-to-Site** — two or more gateways routing between LANs
3. **Hub with Peer Routing** — hub-and-spoke where peers can reach each other through the hub

## Rationale

- Mesh requires every peer to know every other peer's endpoint and public key. This means O(n²) configuration updates and key distribution — fundamentally different from hub-and-spoke where only the server needs to know all peers.
- Mesh peers behind NAT need a coordination service (STUN/TURN equivalent) for hole-punching. This adds significant complexity and an external dependency.
- Tools like Tailscale and Netbird already solve mesh networking well. wgpilot targets a different use case: simple, self-hosted VPN gateway and site-to-site management.
- The "Hub with Peer Routing" mode covers the most common mesh use case (peers reaching each other) without mesh complexity — traffic routes through the hub.

## Consequences

- Users who need true peer-to-peer mesh should use Tailscale, Netbird, or similar.
- All traffic between peers in "Hub with Peer Routing" mode traverses the server, adding latency and bandwidth load compared to direct mesh connections.
- The data model and API are simpler: peers only need the server's public key and endpoint, not every other peer's.

---

## All Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language | Go | Single binary, no runtime deps, wgctrl-go ecosystem |
| Frontend | React + shadcn/ui | Best component library for management UIs |
| Database | SQLite (pure Go) | Zero config, file-based, no CGO |
| WG management | wgctrl-go + netlink | No shell-outs, direct kernel control |
| Firewall | google/nftables | Programmatic, dedicated table avoids conflicts |
| TLS | Built-in autocert | No nginx/Caddy dependency |
| Service | systemd | Standard Linux, hardened unit file |
| Auth | bcrypt + JWT cookie | Simple, secure, no external auth deps |
| Distribution | GoReleaser + GitHub Releases | Automated cross-compilation |
| Config storage | SQLite (runtime) + YAML (static) | DB for dynamic config, file for deployment config |
| Logging | slog (stdlib) | Structured JSON, no external dependency |
| Metrics | Prometheus exposition | Industry standard, optional for users |
| Live updates | SSE | Simpler than WebSocket for one-directional data |
| Private key storage | Encrypted at rest (AES-256-GCM) | Defense in depth |
| Naming | wgpilot | Short, memorable, available |
| License | MIT | Standard for WG ecosystem |
