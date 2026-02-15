# feat-002: Split-Tunnel / Policy Routing

> **Status:** Proposed
> **Priority:** Tier 1 — High Impact
> **Effort:** High
> **Library:** vishvananda/netlink (ip rules) + wgctrl-go (FirewallMark)
> **Unique:** Mostly — no other lightweight WG manager exposes policy routing

---

## Motivation

wgpilot currently manages AllowedIPs on peers but does **not** manage the host routing table or ip rules. This means:

1. Traffic from the server to peer subnets may not route correctly without manual `ip route` commands
2. Split-tunnel configurations (route only specific traffic through WG) require manual policy routing
3. There's no way to use different routing tables per network or per peer
4. The `FirewallMark` field on WireGuard devices is unused — it enables mark-based policy routing

Adding policy routing via netlink turns wgpilot from a "WireGuard UI" into a "WireGuard router."

## User Stories

- **Remote worker**: "Route only my company's 10.0.0.0/8 through the VPN, leave everything else direct."
- **MSP admin**: "Each client network should use a separate routing table to prevent cross-contamination."
- **Privacy user**: "Route all traffic except local LAN through the VPN."
- **Multi-homed server**: "Route VPN traffic through eth0 but not eth1."

## Design

### Data Model Changes

**Migration:** `005_policy_routing.sql`

```sql
-- Per-network routing configuration
ALTER TABLE networks ADD COLUMN routing_table INTEGER DEFAULT 0;
ALTER TABLE networks ADD COLUMN firewall_mark INTEGER DEFAULT 0;

-- Per-network route entries (server-side routes, not peer AllowedIPs)
CREATE TABLE IF NOT EXISTS network_routes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id  INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    destination TEXT NOT NULL,           -- CIDR: "10.0.0.0/8"
    gateway     TEXT DEFAULT '',         -- next-hop IP (empty = direct)
    metric      INTEGER DEFAULT 100,
    description TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_network_routes_network ON network_routes(network_id);

-- Policy routing rules
CREATE TABLE IF NOT EXISTS routing_rules (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id  INTEGER NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    priority    INTEGER NOT NULL DEFAULT 100,
    source      TEXT DEFAULT '',         -- source CIDR match
    destination TEXT DEFAULT '',         -- destination CIDR match
    firewall_mark INTEGER DEFAULT 0,    -- fwmark match
    table_id    INTEGER NOT NULL,        -- routing table to use
    description TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_routing_rules_network ON routing_rules(network_id);
```

### API Changes

**Existing endpoints — new fields on network:**

```json
{
  "name": "office-vpn",
  "routing_table": 100,
  "firewall_mark": 100
}
```

**New endpoints:**

```
GET    /api/networks/{id}/routes       — list routes for a network
POST   /api/networks/{id}/routes       — add a route
DELETE /api/networks/{id}/routes/{rid}  — remove a route

GET    /api/networks/{id}/rules        — list policy rules for a network
POST   /api/networks/{id}/rules        — add a policy rule
DELETE /api/networks/{id}/rules/{rid}   — remove a policy rule
```

**Route request/response:**

```json
{
  "destination": "10.0.0.0/8",
  "gateway": "",
  "metric": 100,
  "description": "Office network via VPN"
}
```

**Rule request/response:**

```json
{
  "priority": 100,
  "source": "10.0.1.0/24",
  "destination": "",
  "firewall_mark": 100,
  "table_id": 100,
  "description": "Route VPN peers through table 100"
}
```

### Kernel Implementation

**Route management:**

```go
// Add route to a specific table
netlink.RouteAdd(&netlink.Route{
    LinkIndex: link.Attrs().Index,
    Dst:       dstNet,          // *net.IPNet
    Gw:        gateway,         // net.IP (or nil for direct)
    Table:     network.RoutingTable,
    Priority:  route.Metric,
    Protocol:  netlink.RTPROT_STATIC,
})
```

**Policy rule management:**

```go
// Add ip rule: source 10.0.1.0/24 → lookup table 100
netlink.RuleAdd(&netlink.Rule{
    Priority: uint32(rule.Priority),
    Src:      srcNet,
    Dst:      dstNet,
    Table:    uint32(rule.TableID),
    Mark:     uint32(rule.FirewallMark),
})
```

**FirewallMark on WireGuard device:**

```go
// Set fwmark on the WireGuard device (currently unused in wgpilot)
mark := network.FirewallMark
wgClient.ConfigureDevice(ifaceName, wgtypes.Config{
    FirewallMark: &mark,
})
```

This causes all packets originating from the WireGuard interface to be marked, enabling mark-based routing rules.

### Package Changes

```
internal/wg/
├── routes.go       — Route management (RouteAdd/Del/List via netlink)
├── rules.go        — Policy rule management (RuleAdd/Del/List via netlink)
└── routes_test.go  — Tests with mocked netlink

internal/server/
├── routes_network_routes.go  — HTTP handlers for /api/networks/{id}/routes
└── routes_network_rules.go   — HTTP handlers for /api/networks/{id}/rules
```

### Reconciliation

On startup, reconciliation must also handle routes and rules:

1. Read desired routes/rules from database
2. Read actual routes/rules from kernel (`netlink.RouteListFiltered`, `netlink.RuleList`)
3. Add missing entries, remove stale entries
4. Log every discrepancy

### Frontend Changes

- **Network detail page**: New "Routing" tab showing routes table and rules table
- **Route form**: Destination CIDR, gateway (optional), metric, description
- **Rule form**: Priority, source CIDR, destination CIDR, fwmark, table ID, description
- **Network form**: Add routing_table and firewall_mark fields (advanced section)

## Implementation Steps

1. Add `routing_table` and `firewall_mark` to networks table (migration)
2. Create `network_routes` and `routing_rules` tables (migration)
3. Add DB CRUD for routes and rules
4. Implement `internal/wg/routes.go` with netlink RouteAdd/Del/List
5. Implement `internal/wg/rules.go` with netlink RuleAdd/Del/List
6. Wire FirewallMark into `toWGConfig` (currently ignored)
7. Add HTTP handlers for routes and rules endpoints
8. Extend reconciliation to cover routes and rules
9. Add frontend routing tab
10. Write integration tests

## Validation

- Route added via API appears in `ip route show table <N>`
- Rule added via API appears in `ip rule show`
- FirewallMark is set on WireGuard device
- Routes/rules survive wgpilot restart (reconciliation)
- Deleting a network cascades to delete routes and rules (DB + kernel)
- Invalid CIDR, negative metric, or duplicate priority return 400

## Cross-References

- [network-management.md](network-management.md) — Network lifecycle (routes/rules applied after interface creation)
- [feat-001-per-peer-bandwidth-limits.md](feat-001-per-peer-bandwidth-limits.md) — FirewallMark could also be used for QoS classification
- [feat-009-route-table-viewer.md](feat-009-route-table-viewer.md) — Route viewer would display managed routes
- [feat-004-event-driven-monitoring.md](feat-004-event-driven-monitoring.md) — RouteSubscribe can detect external route changes
- [architecture/data-model.md](../architecture/data-model.md) — Schema changes
- [architecture/api-surface.md](../architecture/api-surface.md) — New endpoints
