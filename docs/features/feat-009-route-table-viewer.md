# feat-009: Route Table Viewer

> **Status:** Proposed
> **Priority:** Tier 3 — Niche but Useful
> **Effort:** Low
> **Library:** vishvananda/netlink (`RouteListFiltered`, `RuleList`)
> **Unique:** No, but essential for debugging routing issues

---

## Motivation

When traffic isn't routing correctly through WireGuard, the first thing an admin checks is the kernel routing table (`ip route show`). Currently, diagnosing routing issues requires SSH access to the server and familiarity with `ip route` and `ip rule` commands.

wgpilot can provide this information directly in the web UI, filtered to show only WireGuard-relevant routes. This is especially valuable for:

- Verifying that AllowedIPs routes are installed correctly
- Detecting conflicting routes from other software
- Debugging split-tunnel configurations
- Validating policy routing rules (if feat-002 is implemented)

## User Stories

- **Sysadmin**: "Traffic to 10.0.0.0/24 isn't routing through the VPN. What does the routing table look like?"
- **Debugger**: "I added a peer with AllowedIPs but the route isn't showing up."
- **Multi-network admin**: "Two networks have overlapping routes. Which one wins?"

## Design

### No Data Model Changes

Routes are read live from the kernel. No database changes needed.

### API Changes

**New endpoints:**

`GET /api/system/routes` — list all routes on WireGuard interfaces:

```json
{
  "routes": [
    {
      "destination": "10.0.0.0/24",
      "gateway": "",
      "interface": "wg0",
      "source": "",
      "metric": 0,
      "table": 254,
      "table_name": "main",
      "protocol": "kernel",
      "scope": "link",
      "type": "unicast",
      "managed": true
    },
    {
      "destination": "10.0.1.0/24",
      "gateway": "10.0.0.2",
      "interface": "wg0",
      "source": "",
      "metric": 100,
      "table": 254,
      "table_name": "main",
      "protocol": "static",
      "scope": "universe",
      "type": "unicast",
      "managed": false
    }
  ]
}
```

Query parameters:
- `?interface=wg0` — filter by interface
- `?table=100` — filter by routing table
- `?all=true` — include non-WireGuard routes (admin only)

`GET /api/system/rules` — list ip rules (policy routing):

```json
{
  "rules": [
    {
      "priority": 0,
      "source": "",
      "destination": "",
      "table": 255,
      "table_name": "local",
      "action": "lookup"
    },
    {
      "priority": 100,
      "source": "10.0.0.0/24",
      "destination": "",
      "table": 100,
      "table_name": "",
      "action": "lookup",
      "managed": true
    },
    {
      "priority": 32766,
      "source": "",
      "destination": "",
      "table": 254,
      "table_name": "main",
      "action": "lookup"
    }
  ]
}
```

### Kernel Implementation

```go
type RouteViewer struct {
    wgIfaces []string // managed WireGuard interface names
    logger   *slog.Logger
}

func (v *RouteViewer) ListWireGuardRoutes() ([]RouteEntry, error) {
    var routes []RouteEntry

    for _, ifName := range v.wgIfaces {
        link, err := netlink.LinkByName(ifName)
        if err != nil {
            continue // Interface may not exist yet
        }

        // Get routes for this interface
        filter := &netlink.Route{
            LinkIndex: link.Attrs().Index,
        }
        ifRoutes, err := netlink.RouteListFiltered(
            netlink.FAMILY_ALL, filter, netlink.RT_FILTER_OIF)
        if err != nil {
            return nil, fmt.Errorf("list routes for %s: %w", ifName, err)
        }

        for _, r := range ifRoutes {
            routes = append(routes, RouteEntry{
                Destination: cidrString(r.Dst),
                Gateway:     ipString(r.Gw),
                Interface:   ifName,
                Source:      ipString(r.Src),
                Metric:      r.Priority,
                Table:       r.Table,
                TableName:   tableIDToName(r.Table),
                Protocol:    protocolName(r.Protocol),
                Scope:       scopeName(r.Scope),
                Type:        routeTypeName(r.Type),
            })
        }
    }

    return routes, nil
}

func (v *RouteViewer) ListRules() ([]RuleEntry, error) {
    rules, err := netlink.RuleList(netlink.FAMILY_ALL)
    if err != nil {
        return nil, fmt.Errorf("list rules: %w", err)
    }

    var entries []RuleEntry
    for _, r := range rules {
        entries = append(entries, RuleEntry{
            Priority:    r.Priority,
            Source:      cidrString(r.Src),
            Destination: cidrString(r.Dst),
            Table:       r.Table,
            TableName:   tableIDToName(int(r.Table)),
            IifName:     r.IifName,
            OifName:     r.OifName,
            Mark:        r.Mark,
        })
    }

    return entries, nil
}

// Helper: map well-known table IDs to names
func tableIDToName(id int) string {
    switch id {
    case 253:
        return "default"
    case 254:
        return "main"
    case 255:
        return "local"
    default:
        return ""
    }
}
```

### Package Layout

```
internal/routing/
├── viewer.go       — RouteViewer with ListWireGuardRoutes, ListRules
├── types.go        — RouteEntry, RuleEntry types
├── helpers.go      — Name resolution helpers (protocol, scope, table, type)
└── viewer_test.go  — Tests
```

### Managed Route Detection

For each route, determine if it's managed by wgpilot (created during interface setup) or externally created:

```go
// A route is "managed" if:
// 1. Its protocol is RTPROT_KERNEL (auto-created for interface addresses)
// 2. It matches a WireGuard peer's AllowedIPs
// 3. It was explicitly added by feat-002 (policy routing)
```

### Frontend Changes

- **System page**: New "Routing" section with two tabs:
  - **Routes tab**: Sortable table of routes (destination, gateway, interface, metric, table)
  - **Rules tab**: Sortable table of ip rules (priority, source, destination, table)
- **Badges**: "Managed" badge for wgpilot-created routes, "External" for others
- **Warnings**: Highlight conflicting routes (same destination, different interfaces)

### Diagnose CLI Integration

Add routing check to `wgpilot diagnose`:

```
=== Routing ===
[PASS] wg0: 4 routes installed (2 managed, 2 external)
[PASS] wg1: 2 routes installed (2 managed)
[WARN] Route conflict: 10.0.0.0/24 via wg0 AND via eth0
[PASS] 3 ip rules configured
```

## Implementation Steps

1. Implement `internal/routing/viewer.go` with route and rule listing
2. Add HTTP handlers for `/api/system/routes` and `/api/system/rules`
3. Add managed route detection logic
4. Add routing check to `wgpilot diagnose`
5. Add routing section to system page in frontend
6. Write tests

## Validation

- `GET /api/system/routes` returns routes for all WireGuard interfaces
- Routes for non-WireGuard interfaces are excluded by default
- `?interface=wg0` filter works correctly
- `GET /api/system/rules` returns all ip rules
- Managed routes are correctly identified
- Empty interfaces (no routes) return empty array, not error

## Cross-References

- [feat-002-split-tunnel-policy-routing.md](feat-002-split-tunnel-policy-routing.md) — Viewer shows policy routes created by feat-002
- [feat-004-event-driven-monitoring.md](feat-004-event-driven-monitoring.md) — Route events trigger viewer refresh
- [network-management.md](network-management.md) — Routes are created during interface setup
- [operations/service.md](../operations/service.md) — Diagnose CLI enhancements
- [architecture/api-surface.md](../architecture/api-surface.md) — New endpoints
