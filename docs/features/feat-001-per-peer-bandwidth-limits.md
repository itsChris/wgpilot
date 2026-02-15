# feat-001: Per-Peer Bandwidth Limits (QoS)

> **Status:** Proposed
> **Priority:** Tier 1 — High Impact
> **Effort:** High
> **Library:** vishvananda/netlink (traffic control subsystem)
> **Unique:** Yes — no other lightweight WireGuard manager offers this

---

## Motivation

Network operators (MSPs, homelab admins, team leads) frequently need to limit how much bandwidth individual peers can consume. Today, a single peer downloading a large file can saturate the WireGuard interface and degrade the experience for everyone else.

Current alternatives:
- Manual `tc` commands on the host (error-prone, doesn't survive reboot)
- External traffic shaping appliances (overkill for most setups)
- No WireGuard-specific tooling exists for this

wgpilot can solve this natively using Linux traffic control (tc) via the netlink API — no shell-outs, fully managed, persisted in the database.

## User Stories

- **MSP admin**: "I want to limit each client's VPN bandwidth to the plan they pay for (10/50/100 Mbps)."
- **Homelab user**: "I want to prevent my phone's backup from saturating my VPN tunnel."
- **Team lead**: "I want fair bandwidth sharing across 20 developer peers."

## Design

### Data Model Changes

**Migration:** `004_peer_bandwidth.sql`

```sql
ALTER TABLE peers ADD COLUMN bandwidth_up_kbps INTEGER DEFAULT 0;
ALTER TABLE peers ADD COLUMN bandwidth_down_kbps INTEGER DEFAULT 0;
```

A value of `0` means unlimited (no shaping applied). Values are in **kbps** (kilobits per second) to support granular limits without floating point.

### API Changes

**Existing endpoints — new fields:**

`POST /api/networks/{id}/peers` and `PUT /api/networks/{id}/peers/{pid}`:

```json
{
  "name": "laptop",
  "bandwidth_up_kbps": 10000,
  "bandwidth_down_kbps": 50000
}
```

Response includes the same fields.

**New endpoint:**

`GET /api/networks/{id}/bandwidth` — returns aggregate and per-peer bandwidth usage vs limits:

```json
{
  "network_id": 1,
  "peers": [
    {
      "peer_id": 5,
      "name": "laptop",
      "bandwidth_up_kbps": 10000,
      "bandwidth_down_kbps": 50000,
      "current_up_kbps": 4200,
      "current_down_kbps": 31000
    }
  ]
}
```

### Kernel Implementation

Traffic shaping uses the **HTB (Hierarchical Token Bucket)** qdisc with per-peer classes:

```
wg0 (interface)
 └── HTB root qdisc (handle 1:)
      ├── Class 1:1 (peer A) — rate 10mbit ceil 10mbit
      │    └── FQ_CoDel leaf qdisc (fair queuing + AQM)
      ├── Class 1:2 (peer B) — rate 50mbit ceil 50mbit
      │    └── FQ_CoDel leaf qdisc
      └── Class 1:ffff (default/unlimited) — rate 1gbit
           └── FQ_CoDel leaf qdisc
```

**Direction handling:**
- **Download limits (server → peer):** Apply HTB on the WireGuard interface egress (outgoing to peer)
- **Upload limits (peer → server):** Apply HTB on the ingress side using an IFB (Intermediate Functional Block) device, redirecting incoming traffic through tc

**Netlink API calls:**

```go
// Create root qdisc
netlink.QdiscAdd(&netlink.Htb{
    QdiscAttrs: netlink.QdiscAttrs{
        LinkIndex: link.Attrs().Index,
        Handle:    netlink.MakeHandle(1, 0),
        Parent:    netlink.HANDLE_ROOT,
    },
    DefCls: 0xffff,
})

// Create per-peer class
netlink.ClassAdd(&netlink.HtbClass{
    ClassAttrs: netlink.ClassAttrs{
        LinkIndex: link.Attrs().Index,
        Handle:    netlink.MakeHandle(1, peerClassID),
        Parent:    netlink.MakeHandle(1, 0),
    },
    Rate:    uint64(peer.BandwidthDownKbps) * 1000 / 8, // kbps → bytes/sec
    Ceil:    uint64(peer.BandwidthDownKbps) * 1000 / 8,
    Buffer:  32 * 1024, // 32KB burst
})

// Flower filter matching peer IP → class
netlink.FilterAdd(&netlink.Flower{
    FilterAttrs: netlink.FilterAttrs{
        LinkIndex: link.Attrs().Index,
        Parent:    netlink.MakeHandle(1, 0),
        Priority:  peerClassID,
    },
    DestIP:  peerIP,
    ClassID: netlink.MakeHandle(1, peerClassID),
})
```

### Package Layout

```
internal/qos/
├── manager.go      — QoSManager struct, Apply/Remove/Update methods
├── htb.go          — HTB qdisc + class management
├── filter.go       — Flower filter management (peer IP → class mapping)
├── ifb.go          — IFB device for ingress shaping
└── manager_test.go — Tests with mocked netlink
```

### Frontend Changes

- **Peer form**: Add "Bandwidth Limit" section with up/down fields (kbps input with Mbps display helper)
- **Peer table**: Show bandwidth limit column (e.g., "10/50 Mbps" or "Unlimited")
- **Network detail**: Bandwidth utilization bar per peer (if stats endpoint is available)

## Implementation Steps

1. Add `bandwidth_up_kbps` and `bandwidth_down_kbps` to peers table (migration)
2. Add fields to DB CRUD operations and API request/response types
3. Implement `internal/qos/manager.go` with HTB + Flower filter setup
4. Wire QoSManager into server — call on peer create/update/delete
5. Add IFB device management for upload limits
6. Add bandwidth fields to peer form in frontend
7. Add bandwidth column to peer table
8. Write integration tests

## Validation

- Peer with 10 Mbps limit cannot exceed 10 Mbps throughput (iperf3 test)
- Peer with 0 (unlimited) has no qdisc class
- Updating bandwidth limit live takes effect within seconds
- Removing bandwidth limit removes the tc class and filter
- Deleting a peer cleans up its tc rules
- Multiple peers on the same interface get independent limits

## Cross-References

- [peer-management.md](peer-management.md) — Peer CRUD lifecycle (bandwidth fields extend peer model)
- [network-management.md](network-management.md) — Interface lifecycle (QoS applied after interface creation)
- [monitoring.md](monitoring.md) — Transfer stats could show bandwidth utilization vs limit
- [feat-003-interface-level-statistics.md](feat-003-interface-level-statistics.md) — Interface stats complement per-peer QoS data
- [architecture/data-model.md](../architecture/data-model.md) — Schema migration for new columns
- [architecture/api-surface.md](../architecture/api-surface.md) — New/modified endpoints
