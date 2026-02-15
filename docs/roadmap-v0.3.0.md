# Roadmap — v0.3.0 (Kernel API Enhancements)

> Leveraging unused capabilities of wgctrl-go and vishvananda/netlink to add features that no other lightweight WireGuard manager offers.
>
> Prerequisite: All v0.2.0 features are complete.

---

## Design Philosophy

wgpilot's core differentiator is **kernel-native management** — no shell-outs. v0.2.0 used a fraction of what the kernel APIs offer. v0.3.0 deepens the integration:

- **wgctrl-go**: Expose `Device.Type` and `Peer.PersistentKeepaliveInterval` (currently ignored)
- **vishvananda/netlink**: Tap into traffic control (QoS), policy routing, conntrack, socket diagnostics, event subscriptions, link statistics, and MTU management
- **google/nftables**: No changes planned for v0.3.0 (already sufficient)

All features follow the same rules: no shell-outs, kernel API only, SQLite as source of truth, dependency injection, proper error wrapping.

---

## Feature Summary

| # | Feature | Library | Effort | Impact | Spec |
|---|---------|---------|--------|--------|------|
| 1 | Per-peer bandwidth limits (QoS) | netlink (tc) | High | Very High | [feat-001](features/feat-001-per-peer-bandwidth-limits.md) |
| 2 | Split-tunnel / policy routing | netlink (rules) + wgctrl (fwmark) | High | High | [feat-002](features/feat-002-split-tunnel-policy-routing.md) |
| 3 | Interface-level statistics | netlink (link stats) | Low | Medium | [feat-003](features/feat-003-interface-level-statistics.md) |
| 4 | Event-driven monitoring | netlink (subscribe) | Medium | Medium | [feat-004](features/feat-004-event-driven-monitoring.md) |
| 5 | MTU management | netlink (LinkSetMTU) | Low | Medium | [feat-005](features/feat-005-mtu-management.md) |
| 6 | Active connection viewer | netlink (conntrack) | Medium | High | [feat-006](features/feat-006-active-connection-viewer.md) |
| 7 | Port conflict detection | netlink (socket diag) | Low | Medium | [feat-007](features/feat-007-port-conflict-detection.md) |
| 8 | PersistentKeepalive display | wgctrl (already read) | Very Low | Low | [feat-008](features/feat-008-persistent-keepalive-display.md) |
| 9 | Route table viewer | netlink (route list) | Low | Low | [feat-009](features/feat-009-route-table-viewer.md) |
| 10 | Device type diagnostics | wgctrl (Device.Type) | Very Low | Low | [feat-010](features/feat-010-device-type-diagnostics.md) |

---

## Implementation Phases

### Phase 1: Quick Wins (1-2 days)

Low-effort, high-value features that can be shipped immediately.

| Feature | Effort | What Changes |
|---------|--------|--------------|
| feat-008: PersistentKeepalive display | 1 hour | Stop discarding value, add to API response, add column to UI |
| feat-010: Device type diagnostics | 1 hour | Read Device.Type, add to status API, add diagnose check |
| feat-005: MTU management | 0.5 day | DB migration, netlink.LinkSetMTU, network form field, config generation |
| feat-007: Port conflict detection | 0.5 day | SocketDiagUDP, pre-validation, diagnose check |

**Touches:** internal/wg/device.go, internal/wg/iface.go, internal/server/routes_status.go, internal/debug/, frontend peer-table, frontend network-form

**Migrations:** 004_network_mtu.sql (feat-005 only)

### Phase 2: Monitoring Enhancements (2-3 days)

Better visibility into what's happening on the network.

| Feature | Effort | What Changes |
|---------|--------|--------------|
| feat-003: Interface-level statistics | 0.5 day | Read link.Attrs().Statistics, add to status API, Prometheus metrics |
| feat-004: Event-driven monitoring | 1-2 days | New internal/monitor/watcher.go, netlink subscriptions, integrate with poller |
| feat-009: Route table viewer | 0.5 day | New internal/routing/, RouteListFiltered, RuleList, new API endpoints |

**Touches:** internal/wg/stats.go (new), internal/monitor/, internal/routing/ (new), internal/server/routes_status.go, Prometheus metrics

**Migrations:** None

### Phase 3: Connection Visibility (2-3 days)

Deep packet-level insight into VPN traffic.

| Feature | Effort | What Changes |
|---------|--------|--------------|
| feat-006: Active connection viewer | 2-3 days | New internal/conntrack/, ConntrackTableList, peer flush on disable/remove, new API endpoints, frontend connections tab |

**Touches:** internal/conntrack/ (new), internal/server/routes_peers.go, internal/server/routes_connections.go (new), frontend network detail page

**Migrations:** None

### Phase 4: Advanced Networking (1-2 weeks)

Major features that significantly expand wgpilot's capabilities.

| Feature | Effort | What Changes |
|---------|--------|--------------|
| feat-001: Per-peer bandwidth limits | 1 week | New internal/qos/, HTB qdiscs, flower filters, IFB device, DB migration, peer form, peer table |
| feat-002: Split-tunnel / policy routing | 1 week | New internal/wg/routes.go + rules.go, DB migration, new tables, new API endpoints, routing UI tab |

**Touches:** internal/qos/ (new), internal/wg/routes.go (new), internal/wg/rules.go (new), internal/server/ (new handlers), internal/db/ (new tables), frontend (new tabs/forms)

**Migrations:** 004_peer_bandwidth.sql (feat-001), 005_policy_routing.sql (feat-002)

---

## Dependency Graph

```
Phase 1 (Quick Wins)
├── feat-008 (keepalive)     ── standalone
├── feat-010 (device type)   ── standalone
├── feat-005 (MTU)           ── standalone
└── feat-007 (port check)    ── standalone

Phase 2 (Monitoring)
├── feat-003 (iface stats)   ── standalone
├── feat-004 (events)        ── enhances monitoring system
└── feat-009 (route viewer)  ── standalone, enhanced by feat-002

Phase 3 (Connections)
└── feat-006 (conntrack)     ── standalone, integrates with peer lifecycle

Phase 4 (Advanced)
├── feat-001 (bandwidth)     ── standalone, uses iface stats from feat-003
└── feat-002 (policy routing)── standalone, displayed by feat-009, events from feat-004
```

No hard dependencies between features — each can be implemented independently. However, the phase ordering reflects logical progression: quick wins first, then monitoring, then visibility, then advanced features.

---

## Database Migrations

| Migration | Feature | SQL |
|-----------|---------|-----|
| 004_network_mtu.sql | feat-005 | `ALTER TABLE networks ADD COLUMN mtu INTEGER DEFAULT 1420` |
| 005_peer_bandwidth.sql | feat-001 | `ALTER TABLE peers ADD COLUMN bandwidth_up_kbps INTEGER DEFAULT 0` + down |
| 006_policy_routing.sql | feat-002 | `CREATE TABLE network_routes (...)` + `CREATE TABLE routing_rules (...)` + network columns |

Migration numbers assume v0.2.0 ends at 003. Adjust if v0.2.0 adds more migrations.

---

## New Packages

| Package | Feature | Purpose |
|---------|---------|---------|
| `internal/qos/` | feat-001 | HTB qdisc + class + flower filter management |
| `internal/conntrack/` | feat-006 | Conntrack table querying and flushing |
| `internal/portcheck/` | feat-007 | UDP/TCP socket diagnostics |
| `internal/routing/` | feat-009 | Route and rule viewer |
| `internal/monitor/watcher.go` | feat-004 | Netlink event subscriptions |

All new packages follow existing patterns: interface-driven, dependency-injected, context-propagated, error-wrapped.

---

## New API Endpoints

| Method | Path | Feature |
|--------|------|---------|
| GET | /api/networks/{id}/bandwidth | feat-001 |
| GET | /api/networks/{id}/routes | feat-002 |
| POST | /api/networks/{id}/routes | feat-002 |
| DELETE | /api/networks/{id}/routes/{rid} | feat-002 |
| GET | /api/networks/{id}/rules | feat-002 |
| POST | /api/networks/{id}/rules | feat-002 |
| DELETE | /api/networks/{id}/rules/{rid} | feat-002 |
| GET | /api/networks/{id}/connections | feat-006 |
| GET | /api/networks/{id}/connections/summary | feat-006 |
| DELETE | /api/networks/{id}/connections | feat-006 |
| GET | /api/system/ports | feat-007 |
| GET | /api/system/routes | feat-009 |
| GET | /api/system/rules | feat-009 |

---

## Success Criteria

v0.3.0 is complete when:

1. All 10 features are implemented, tested, and documented
2. `go test -race ./...` passes
3. `wgpilot diagnose` includes checks for MTU, port conflicts, device type, and route health
4. Prometheus metrics include interface-level counters
5. Frontend displays: bandwidth limits, connections, routes, keepalive, device type, MTU
6. README updated with new features
7. No regressions in existing v0.2.0 functionality
