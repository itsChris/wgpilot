# Network Management

> **Purpose**: Specifies network CRUD operations, the three topology modes (gateway, site-to-site, hub-routed), WireGuard interface lifecycle, nftables rule templates, IP allocation, and reconciliation logic.
>
> **Related docs**: [../architecture/data-model.md](../architecture/data-model.md), [multi-network.md](multi-network.md), [peer-management.md](peer-management.md)
>
> **Implements**: `internal/wg/`, `internal/nft/`, `internal/server/handlers/networks.go`

---

## WireGuard Management Library Stack

```
┌─────────────────────────────────────┐
│          wg/manager.go              │  High-level operations
│  CreateNetwork, AddPeer, Remove...  │
├─────────────────────────────────────┤
│          wg/device.go               │  wgctrl-go
│  ConfigureDevice, GetDevice         │  (WireGuard kernel API)
├─────────────────────────────────────┤
│          wg/iface.go                │  vishvananda/netlink
│  LinkAdd, AddrAdd, LinkSetUp        │  (Network interface mgmt)
├─────────────────────────────────────┤
│          wg/route.go                │  vishvananda/netlink
│  RouteAdd, RouteReplace             │  (Routing table mgmt)
├─────────────────────────────────────┤
│          nft/manager.go             │  google/nftables
│  AddMasquerade, AddForward          │  (Firewall rules)
└─────────────────────────────────────┘
```

## Key Operations

### Create Network

1. Generate server keypair (`wgtypes.GeneratePrivateKey()`).
2. Create WireGuard interface via netlink (`&netlink.Wireguard{}`).
3. Assign IP address from subnet (first usable IP, e.g., 10.0.0.1/24).
4. Configure WireGuard device (private key, listen port).
5. Bring interface up.
6. If NAT enabled: add nftables masquerade rule on POSTROUTING.
7. If inter-peer routing: add nftables forward rule for the subnet.
8. Persist to database.

### Delete Network

1. Remove all nftables rules for this interface.
2. Bring interface down.
3. Delete interface via netlink.
4. Delete from database (cascade deletes peers).

## IP Allocation

Automatic IP allocation from network subnet:

```go
func (m *Manager) AllocateIP(networkID int64) (net.IP, error) {
    network, _ := m.db.GetNetwork(networkID)
    _, subnet, _ := net.ParseCIDR(network.Subnet)

    // Get all allocated IPs in this network
    peers, _ := m.db.ListPeers(networkID)
    used := map[string]bool{}
    for _, p := range peers {
        ip, _, _ := net.ParseCIDR(p.AllowedIPs)
        used[ip.String()] = true
    }

    // Skip network address, server address (first usable), and broadcast
    serverIP := firstUsableIP(subnet)  // e.g., 10.0.0.1
    used[serverIP.String()] = true

    // Find next available
    for ip := nextIP(serverIP); subnet.Contains(ip); ip = nextIP(ip) {
        if !isBroadcast(ip, subnet) && !used[ip.String()] {
            return ip, nil
        }
    }

    return nil, fmt.Errorf("no available IPs in subnet %s", network.Subnet)
}
```

## Reconciliation Logic

On startup, the app reconciles kernel state against the database. The database is always the source of truth.

```go
func (m *Manager) Reconcile(ctx context.Context) error {
    // 1. Get all networks from DB
    networks, _ := m.db.ListNetworks()

    // 2. Get all existing WG interfaces from kernel
    devices, _ := m.wgClient.Devices()
    existingIfaces := map[string]bool{}
    for _, d := range devices {
        existingIfaces[d.Name] = true
    }

    // 3. For each DB network, ensure kernel matches
    for _, net := range networks {
        if !net.Enabled {
            // If interface exists but shouldn't, tear it down
            if existingIfaces[net.Interface] {
                m.teardownInterface(net.Interface)
            }
            continue
        }

        // Create or reconfigure interface
        m.ensureInterface(net)

        // Sync peers
        dbPeers, _ := m.db.ListPeers(net.ID)
        m.syncPeers(net.Interface, dbPeers)

        // Apply firewall rules
        m.nft.ApplyRules(net)
    }

    // 4. Remove orphaned interfaces (in kernel but not in DB)
    for _, d := range devices {
        if !m.db.InterfaceExists(d.Name) {
            m.teardownInterface(d.Name)
        }
    }

    return nil
}
```

## Network Topology Modes

### Mode 1: VPN Gateway (default)

The server acts as a NAT gateway. Clients route all (or some) traffic through the server.

| Setting | Server-side | Client config |
|---|---|---|
| AllowedIPs (on server) | `peer_ip/32` | — |
| AllowedIPs (in .conf) | — | `0.0.0.0/0, ::/0` (full tunnel) or specific CIDRs (split tunnel) |
| NAT | masquerade wg→eth | — |
| IP forwarding | wg→eth only | — |
| nftables | `MASQUERADE` on `POSTROUTING` for wg interface | — |

### Mode 2: Site-to-Site

Two or more gateways, each representing a local LAN. Traffic routes between sites.

| Setting | Server-side | Client config |
|---|---|---|
| AllowedIPs (on server) | `peer_ip/32` + `site_networks` CIDRs | — |
| AllowedIPs (in .conf) | — | Remote site subnets |
| NAT | No (unless explicitly toggled) | — |
| IP forwarding | wg↔wg + wg↔eth | — |
| nftables | `FORWARD` between subnets | — |

### Mode 3: Hub with Peer Routing

Same as gateway but clients can reach each other through the hub.

| Setting | Server-side | Client config |
|---|---|---|
| AllowedIPs (on server) | `peer_ip/32` | — |
| AllowedIPs (in .conf) | — | WG subnet (`10.0.0.0/24`) |
| NAT | No | — |
| IP forwarding | wg↔wg only | — |
| nftables | `FORWARD` within wg subnet | — |

## nftables Rule Templates

```go
// Gateway mode: NAT
func (n *NFTManager) AddMasquerade(iface string, subnet string) {
    // table ip wgpilot
    //   chain postrouting { type nat hook postrouting priority 100 }
    //     iifname "wg0" oifname != "wg0" masquerade
}

// Hub-routed mode: inter-peer forwarding
func (n *NFTManager) AddInterPeerForward(iface string) {
    // table ip wgpilot
    //   chain forward { type filter hook forward priority 0 }
    //     iifname "wg0" oifname "wg0" accept
}

// Site-to-site: forward between subnets
func (n *NFTManager) AddSubnetForward(iface string, localSubnet, remoteSubnet string) {
    // table ip wgpilot
    //   chain forward { type filter hook forward priority 0 }
    //     ip saddr <localSubnet> ip daddr <remoteSubnet> accept
    //     ip saddr <remoteSubnet> ip daddr <localSubnet> accept
}
```

All rules are managed in a dedicated `wgpilot` nftables table to avoid conflicts with existing firewall rules.

---

## v0.3.0 Enhancements (Proposed)

The following proposed features extend network management:

- [feat-001: Per-Peer Bandwidth Limits](feat-001-per-peer-bandwidth-limits.md) — QoS via HTB qdiscs on WireGuard interfaces (per-peer upload/download limits)
- [feat-002: Split-Tunnel / Policy Routing](feat-002-split-tunnel-policy-routing.md) — Managed ip rules, routing tables, and FirewallMark for advanced routing
- [feat-005: MTU Management](feat-005-mtu-management.md) — Per-network MTU configuration via `netlink.LinkSetMTU`
- [feat-007: Port Conflict Detection](feat-007-port-conflict-detection.md) — Pre-validate UDP port availability via socket diagnostics before interface creation
