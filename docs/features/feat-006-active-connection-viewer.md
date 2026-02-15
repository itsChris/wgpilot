# feat-006: Active Connection Viewer (Conntrack)

> **Status:** Proposed
> **Priority:** Tier 2 — Medium Impact
> **Effort:** Medium
> **Library:** vishvananda/netlink (conntrack subsystem)
> **Unique:** Yes — no WireGuard management tool exposes conntrack data

---

## Motivation

When debugging WireGuard connectivity, the most common question is: "Is traffic actually flowing through the tunnel?" Today, wgpilot shows transfer byte counters and last handshake time, but this doesn't tell you **what** traffic is flowing — which connections, to which destinations, using which protocols.

Linux's conntrack (connection tracking) subsystem already tracks every connection flowing through the system. By filtering conntrack entries for WireGuard interface IPs, wgpilot can provide a real-time view of active connections — like a "netstat for your VPN."

Additionally, cleaning up stale conntrack entries when a peer is disabled or removed prevents ghost connections and NAT table bloat.

## User Stories

- **Sysadmin**: "Peer says they're connected but can't reach anything. Is traffic actually flowing?"
- **MSP admin**: "I want to see what services each client peer is accessing through the VPN."
- **Security reviewer**: "Show me all active connections through the VPN — are there unexpected destinations?"
- **Debugger**: "After removing a peer, stale conntrack entries are causing NAT issues."

## Design

### No Data Model Changes

Conntrack data is ephemeral kernel state — it's read live, not stored. No database changes needed.

### API Changes

**New endpoints:**

`GET /api/networks/{id}/connections` — list active connections through a network:

```json
{
  "network_id": 1,
  "interface": "wg0",
  "total": 47,
  "connections": [
    {
      "protocol": "tcp",
      "state": "ESTABLISHED",
      "source_ip": "10.0.0.2",
      "source_port": 54321,
      "dest_ip": "93.184.216.34",
      "dest_port": 443,
      "bytes_original": 15234,
      "bytes_reply": 892345,
      "packets_original": 42,
      "packets_reply": 38,
      "timeout": 432000,
      "peer_name": "laptop"
    }
  ]
}
```

Query parameters:
- `?peer_id=5` — filter by peer
- `?protocol=tcp` — filter by protocol (tcp, udp, icmp)
- `?limit=100` — pagination limit

`GET /api/networks/{id}/connections/summary` — aggregated per-peer stats:

```json
{
  "network_id": 1,
  "peers": [
    {
      "peer_id": 5,
      "name": "laptop",
      "tcp_connections": 23,
      "udp_flows": 8,
      "total_bytes": 1234567,
      "top_destinations": [
        {"ip": "93.184.216.34", "port": 443, "connections": 5},
        {"ip": "10.0.1.50", "port": 22, "connections": 3}
      ]
    }
  ]
}
```

`DELETE /api/networks/{id}/connections?peer_id=5` — flush connections for a peer (admin only):

### Kernel Implementation

```go
type ConntrackViewer struct {
    logger *slog.Logger
}

func (v *ConntrackViewer) ListConnections(networkSubnet *net.IPNet) ([]Connection, error) {
    flows, err := netlink.ConntrackTableList(netlink.ConntrackTable, unix.AF_INET)
    if err != nil {
        return nil, fmt.Errorf("list conntrack: %w", err)
    }

    var connections []Connection
    for _, flow := range flows {
        // Filter: source or destination is within WireGuard subnet
        srcInSubnet := networkSubnet.Contains(flow.Forward.SrcIP)
        dstInSubnet := networkSubnet.Contains(flow.Forward.DstIP)

        if !srcInSubnet && !dstInSubnet {
            continue
        }

        connections = append(connections, Connection{
            Protocol:       protoName(flow.Forward.Protocol),
            State:          flow.ProtoInfo.State(),
            SourceIP:       flow.Forward.SrcIP.String(),
            SourcePort:     flow.Forward.SrcPort,
            DestIP:         flow.Forward.DstIP.String(),
            DestPort:       flow.Forward.DstPort,
            BytesOriginal:  flow.Forward.Bytes,
            BytesReply:     flow.Reverse.Bytes,
            PacketsOriginal: flow.Forward.Packets,
            PacketsReply:   flow.Reverse.Packets,
            Timeout:        flow.TimeOut,
        })
    }

    return connections, nil
}
```

**Peer connection cleanup on disable/remove:**

```go
func (v *ConntrackViewer) FlushPeerConnections(peerIP net.IP) (int, error) {
    filter := &netlink.ConntrackFilter{}
    filter.AddIP(netlink.ConntrackOrigSrcIP, peerIP)

    deleted, err := netlink.ConntrackDeleteFilters(
        netlink.ConntrackTable, unix.AF_INET, filter)
    if err != nil {
        return 0, fmt.Errorf("flush conntrack for %s: %w", peerIP, err)
    }

    // Also flush reverse direction
    filterRev := &netlink.ConntrackFilter{}
    filterRev.AddIP(netlink.ConntrackOrigDstIP, peerIP)
    deletedRev, _ := netlink.ConntrackDeleteFilters(
        netlink.ConntrackTable, unix.AF_INET, filterRev)

    return deleted + deletedRev, nil
}
```

### Package Layout

```
internal/conntrack/
├── viewer.go       — ConntrackViewer with List/Flush methods
├── types.go        — Connection, ConnectionSummary types
└── viewer_test.go  — Tests
```

### Integration Points

**Peer removal** (`internal/server/routes_peers.go`):

```go
// In handleDeletePeer, after removing from WireGuard:
flushed, err := s.conntrack.FlushPeerConnections(peerIP)
if err != nil {
    s.logger.Warn("failed to flush conntrack", "peer_ip", peerIP, "error", err)
    // Non-fatal: peer is already removed
}
s.logger.Info("flushed peer connections", "peer_ip", peerIP, "count", flushed)
```

**Peer disable** (same integration point as removal).

### Frontend Changes

- **Network detail page**: New "Connections" tab showing active connections table
  - Columns: Protocol, Source, Destination, Bytes, Packets, State, Peer
  - Filters: by peer, by protocol
  - Auto-refresh every 10s
- **Peer detail**: Connection count badge
- **Admin action**: "Flush connections" button per peer (calls DELETE endpoint)

### Performance Considerations

- Conntrack tables can have 100k+ entries on busy systems
- Filtering by subnet happens in Go, not in the kernel
- Use `limit` parameter and pagination for large tables
- Cache results for 5 seconds to avoid hammering the kernel
- Consider streaming via SSE for real-time connection monitoring

## Implementation Steps

1. Implement `internal/conntrack/viewer.go` with `ListConnections` and `FlushPeerConnections`
2. Add peer-to-IP mapping helper (resolve peer name from IP)
3. Add HTTP handlers for connections endpoints
4. Wire `FlushPeerConnections` into peer disable and delete handlers
5. Add connections tab to network detail page in frontend
6. Add connection count badge to peer table
7. Write tests

## Validation

- `GET /api/networks/{id}/connections` returns active connections matching WireGuard subnet
- Filtering by `?peer_id=5` returns only that peer's connections
- `DELETE /api/networks/{id}/connections?peer_id=5` flushes entries from conntrack
- Disabling a peer automatically flushes its conntrack entries
- Empty result when no traffic is flowing (not an error)
- Performance: 10k conntrack entries filtered in < 100ms

## Cross-References

- [monitoring.md](monitoring.md) — Extends monitoring with connection-level visibility
- [peer-management.md](peer-management.md) — Peer disable/remove triggers conntrack flush
- [feat-003-interface-level-statistics.md](feat-003-interface-level-statistics.md) — Interface stats show aggregate, connections show detail
- [feat-007-port-conflict-detection.md](feat-007-port-conflict-detection.md) — Socket diagnostics complement conntrack for debugging
- [architecture/api-surface.md](../architecture/api-surface.md) — New endpoints
