# feat-005: MTU Management

> **Status:** Proposed
> **Priority:** Tier 2 — Medium Impact
> **Effort:** Low
> **Library:** vishvananda/netlink (`LinkSetMTU`)
> **Unique:** No, but prevents the #1 WireGuard troubleshooting issue

---

## Motivation

MTU (Maximum Transmission Unit) misconfiguration is the most common cause of "WireGuard connects but nothing works" or "large packets fail while small ones succeed." The default WireGuard MTU is 1420 bytes (1500 ethernet - 80 bytes WireGuard overhead), but this value is wrong in many scenarios:

- **PPPoE connections**: Path MTU is 1492, so WireGuard MTU should be 1412
- **Double encapsulation** (WG over WG, WG over VPN): Need further reduction
- **IPv6**: WireGuard overhead is 80 bytes (vs 60 for IPv4)
- **Jumbo frames**: Datacenters with 9000-byte MTU can use higher WireGuard MTU

wgpilot currently creates interfaces with the kernel default MTU and provides no way to configure it. The fix is a single netlink call (`LinkSetMTU`), but it needs to be exposed in the UI with proper validation and guidance.

## User Stories

- **Home user on PPPoE**: "My VPN connects but web pages load slowly or timeout — it's an MTU issue."
- **Datacenter admin**: "My WireGuard interface should use MTU 8920 to take advantage of jumbo frames."
- **Nested VPN user**: "I'm running WireGuard over another VPN and need to reduce MTU further."

## Design

### Data Model Changes

**Migration:** `006_network_mtu.sql`

```sql
ALTER TABLE networks ADD COLUMN mtu INTEGER DEFAULT 1420;
```

### API Changes

**Existing endpoints — new field on network:**

`POST /api/networks` and `PUT /api/networks/{id}`:

```json
{
  "name": "office",
  "mtu": 1420
}
```

Response includes the `mtu` field.

### Validation Rules

| Rule | Value | Rationale |
|------|-------|-----------|
| Minimum | 1280 | IPv6 minimum MTU requirement |
| Maximum | 9000 | Jumbo frame maximum |
| Default | 1420 | Standard Ethernet (1500) - WireGuard overhead (80) |
| Warning | < 1280 | Below IPv6 minimum, dual-stack will break |
| Warning | > 1500 | Requires jumbo frame support on underlying network |

### Kernel Implementation

```go
// In Manager.CreateInterface, after LinkAdd and AddrAdd:
func (m *Manager) setMTU(ifaceName string, mtu int) error {
    link, err := netlink.LinkByName(ifaceName)
    if err != nil {
        return fmt.Errorf("get link %s for MTU: %w", ifaceName, err)
    }
    if err := netlink.LinkSetMTU(link, mtu); err != nil {
        return fmt.Errorf("set MTU %d on %s: %w", mtu, ifaceName, err)
    }
    return nil
}
```

**Integration point in CreateInterface:**

```go
func (m *Manager) CreateInterface(ctx context.Context, cfg InterfaceConfig) error {
    // 1. Create WireGuard link
    m.link.CreateWireGuardLink(cfg.Name)
    // 2. Set MTU (NEW)
    m.link.SetMTU(cfg.Name, cfg.MTU)
    // 3. Add address
    m.link.AddAddress(cfg.Name, cfg.Address)
    // 4. Configure WireGuard
    m.wg.ConfigureDevice(cfg.Name, wgConfig)
    // 5. Bring up
    m.link.SetLinkUp(cfg.Name)
}
```

**MTU change on existing interface (PUT /api/networks/{id}):**

```go
// MTU can be changed on a live interface without disruption
func (m *Manager) UpdateMTU(ctx context.Context, ifaceName string, mtu int) error {
    link, err := netlink.LinkByName(ifaceName)
    if err != nil {
        return fmt.Errorf("get link %s: %w", ifaceName, err)
    }
    return netlink.LinkSetMTU(link, mtu)
}
```

### Package Changes

Add `SetMTU(name string, mtu int) error` to the `LinkManager` interface in `internal/wg/iface.go`.

### Reconciliation

Add MTU check to startup reconciliation:

```go
// In reconcile.go
link, _ := netlink.LinkByName(ifaceName)
actualMTU := link.Attrs().MTU
if actualMTU != network.MTU {
    log.Warn("MTU mismatch", "interface", ifaceName,
        "expected", network.MTU, "actual", actualMTU)
    netlink.LinkSetMTU(link, network.MTU)
}
```

### Frontend Changes

- **Network form**: Add MTU field with:
  - Input: number, default 1420
  - Validation: 1280-9000
  - Helper text: "Default 1420. Lower if on PPPoE (1412) or nested VPN. Higher for jumbo frames."
  - Warning badge if value is unusual (< 1380 or > 1500)
- **Network detail**: Display current MTU
- **Diagnostics**: Flag MTU mismatches between DB and kernel

### Peer Config Generation

The peer `.conf` file should include the MTU:

```ini
[Interface]
PrivateKey = ...
Address = 10.0.0.2/24
DNS = 1.1.1.1
MTU = 1420
```

Currently `MTU` is not included in generated configs. Add it from the network's MTU value.

## Implementation Steps

1. Add `mtu` column to networks table (migration)
2. Add `MTU` field to DB network model and CRUD
3. Add `SetMTU` to `LinkManager` interface
4. Implement `SetMTU` via `netlink.LinkSetMTU`
5. Call `SetMTU` in `CreateInterface` and `UpdateNetwork`
6. Add MTU to reconciliation checks
7. Add MTU to peer config generation
8. Add MTU field to network form in frontend
9. Add input validation (1280-9000)
10. Write tests

## Validation

- Creating a network with `mtu: 1412` results in `ip link show wg0` showing `mtu 1412`
- Updating MTU on a live interface takes effect without disrupting existing connections
- Reconciliation detects and corrects external MTU changes
- Generated peer configs include `MTU = <value>`
- API rejects MTU < 1280 or > 9000 with 400 error
- Default MTU (when not specified) is 1420

## Cross-References

- [network-management.md](network-management.md) — Interface lifecycle (MTU set during creation)
- [peer-management.md](peer-management.md) — Config generation includes MTU
- [feat-003-interface-level-statistics.md](feat-003-interface-level-statistics.md) — RX errors may indicate MTU issues
- [feat-010-device-type-diagnostics.md](feat-010-device-type-diagnostics.md) — Diagnostics should check MTU alignment
- [architecture/data-model.md](../architecture/data-model.md) — Schema migration
