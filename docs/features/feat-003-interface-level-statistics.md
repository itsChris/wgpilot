# feat-003: Interface-Level Statistics

> **Status:** Proposed
> **Priority:** Tier 1 — High Impact
> **Effort:** Low
> **Library:** vishvananda/netlink (`Link.Attrs().Statistics`)
> **Unique:** No, but currently missing from wgpilot

---

## Motivation

wgpilot currently collects per-peer statistics via wgctrl (ReceiveBytes, TransmitBytes, LastHandshakeTime). However, it has **no interface-level statistics**. The Linux kernel tracks rich counters at the interface level that provide insight no per-peer aggregation can:

- **RX/TX errors** — indicates MTU issues, packet corruption, or driver problems
- **RX/TX drops** — indicates queue overflow or resource exhaustion
- **Multicast packets** — unexpected multicast traffic on a WireGuard interface
- **Collisions** — shouldn't happen on WireGuard but useful for diagnostics
- **Total throughput** — aggregate across all peers, including overhead

These counters are already maintained by the kernel at zero cost. Reading them via netlink is a single syscall.

## User Stories

- **Sysadmin**: "I see high per-peer transfer but my network feels slow — are packets being dropped at the interface level?"
- **Debugger**: "I'm getting RX errors on wg0 — is it an MTU issue?"
- **Dashboard viewer**: "I want to see total interface throughput, not just per-peer."

## Design

### No Data Model Changes

Interface statistics are read live from the kernel. No new database tables needed. Optionally, snapshots could be stored alongside peer snapshots for historical analysis.

### API Changes

**Extended response on existing endpoints:**

`GET /api/status` — add interface-level stats:

```json
{
  "networks": [
    {
      "id": 1,
      "name": "office",
      "interface": "wg0",
      "interface_stats": {
        "rx_bytes": 1234567890,
        "tx_bytes": 9876543210,
        "rx_packets": 1000000,
        "tx_packets": 950000,
        "rx_errors": 0,
        "tx_errors": 0,
        "rx_dropped": 12,
        "tx_dropped": 0,
        "multicast": 0
      },
      "peers": [...]
    }
  ]
}
```

**New Prometheus metrics:**

```
wg_interface_rx_bytes_total{network="office",interface="wg0"} 1234567890
wg_interface_tx_bytes_total{network="office",interface="wg0"} 9876543210
wg_interface_rx_packets_total{network="office",interface="wg0"} 1000000
wg_interface_tx_packets_total{network="office",interface="wg0"} 950000
wg_interface_rx_errors_total{network="office",interface="wg0"} 0
wg_interface_tx_errors_total{network="office",interface="wg0"} 0
wg_interface_rx_dropped_total{network="office",interface="wg0"} 12
wg_interface_tx_dropped_total{network="office",interface="wg0"} 0
```

### Kernel Implementation

```go
func (m *Manager) InterfaceStats(ifaceName string) (*InterfaceStats, error) {
    link, err := netlink.LinkByName(ifaceName)
    if err != nil {
        return nil, fmt.Errorf("get link %s: %w", ifaceName, err)
    }

    stats := link.Attrs().Statistics
    if stats == nil {
        return nil, fmt.Errorf("no statistics for %s", ifaceName)
    }

    return &InterfaceStats{
        RxBytes:   stats.RxBytes,
        TxBytes:   stats.TxBytes,
        RxPackets: stats.RxPackets,
        TxPackets: stats.TxPackets,
        RxErrors:  stats.RxErrors,
        TxErrors:  stats.TxErrors,
        RxDropped: stats.RxDropped,
        TxDropped: stats.TxDropped,
        Multicast: stats.Multicast,
    }, nil
}
```

The `LinkStatistics` struct from netlink provides both 32-bit and 64-bit counters. Use the 64-bit variants to avoid overflow on high-throughput interfaces.

### Package Changes

```
internal/wg/
├── stats.go       — InterfaceStats type + retrieval via netlink
└── stats_test.go  — Tests
```

### Frontend Changes

- **Dashboard**: Add interface throughput card showing total RX/TX across all interfaces
- **Network detail**: Show interface stats panel (bytes, packets, errors, drops)
- **Alerts**: Optionally trigger an alert if RX errors or drops exceed a threshold (extends alert system)

### Prometheus Integration

Add counters to the existing metrics endpoint. These are gauge-type metrics (absolute counters from the kernel, not deltas).

## Implementation Steps

1. Add `InterfaceStats` struct to `internal/wg/stats.go`
2. Implement `InterfaceStats()` method using `netlink.LinkByName` + `link.Attrs().Statistics`
3. Extend status handler to include interface stats in response
4. Add Prometheus gauge metrics for interface counters
5. Add interface stats panel to network detail page in frontend
6. Add interface throughput card to dashboard
7. Write tests

## Validation

- `GET /api/status` returns `interface_stats` with non-zero values when traffic is flowing
- Prometheus endpoint exposes all 8 interface counter metrics
- Stats update on every poll cycle (30s)
- Works correctly with multiple interfaces (wg0, wg1, ...)
- Returns graceful error if interface doesn't exist

## Cross-References

- [monitoring.md](monitoring.md) — Extends the monitoring system with interface-level data
- [feat-001-per-peer-bandwidth-limits.md](feat-001-per-peer-bandwidth-limits.md) — Interface stats complement per-peer QoS data
- [feat-005-mtu-management.md](feat-005-mtu-management.md) — RX errors often indicate MTU issues
- [feat-010-device-type-diagnostics.md](feat-010-device-type-diagnostics.md) — Diagnostics should flag non-zero error counters
- [architecture/api-surface.md](../architecture/api-surface.md) — Extended response format
