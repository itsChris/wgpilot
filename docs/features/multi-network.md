# Multi-Network Support

> **Purpose**: Specifies support for multiple independent WireGuard interfaces, network isolation, inter-network bridging, interface naming, and subnet validation.
>
> **Related docs**: [network-management.md](network-management.md), [../architecture/data-model.md](../architecture/data-model.md)
>
> **Implements**: `internal/wg/manager.go`, `internal/nft/manager.go`, `internal/server/handlers/bridges.go`

---

## Overview

Multiple independent WireGuard interfaces on the same server. Each network gets:

- Its own interface (`wg0`, `wg1`, `wg2`, ...)
- Its own subnet (must not overlap)
- Its own listen port (must be unique)
- Its own server keypair
- Its own nftables rules

Networks are isolated by default. Optional bridging via `network_bridges` table (see [../architecture/data-model.md](../architecture/data-model.md)) allows controlled routing between networks.

## Interface Naming

Auto-assigned: `wg0`, `wg1`, etc. The next available name is chosen on network creation:

```go
func (m *Manager) NextInterfaceName() string {
    existing := m.db.ListInterfaceNames()  // ["wg0", "wg2"]
    for i := 0; ; i++ {
        name := fmt.Sprintf("wg%d", i)
        if !contains(existing, name) {
            return name
        }
    }
}
```

## Subnet Validation

On network creation, validate that the new subnet doesn't overlap with:
- Existing WireGuard network subnets
- The server's physical network interfaces
- Common reserved ranges that would cause conflicts

## Network Bridging

Bridges allow controlled routing between two wgpilot networks. See [../architecture/data-model.md](../architecture/data-model.md) for the `network_bridges` table schema.

Bridge properties:
- **Direction:** `a_to_b`, `b_to_a`, or `bidirectional`
- **Allowed CIDRs:** Optional fine-grained filter for which traffic crosses the bridge
- **Enable/disable:** Toggle without deleting the bridge configuration

When a bridge is created or modified, the nft manager applies appropriate `FORWARD` chain rules in the `wgpilot` nftables table to allow traffic between the two WireGuard interfaces.

## API Endpoints

```
GET    /api/bridges                 # list all bridges
POST   /api/bridges                 # create bridge between networks
GET    /api/bridges/:id             # get bridge details
PUT    /api/bridges/:id             # update bridge
DELETE /api/bridges/:id             # delete bridge
```

See [../architecture/api-surface.md](../architecture/api-surface.md) for full endpoint definitions.
