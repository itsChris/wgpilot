# feat-008: PersistentKeepalive Display

> **Status:** Proposed
> **Priority:** Tier 3 — Quick Win
> **Effort:** Very Low
> **Library:** wgctrl-go (`Peer.PersistentKeepaliveInterval` — already read but discarded)
> **Unique:** No, but fixes a confusing gap in the current UI

---

## Motivation

wgpilot already reads `PersistentKeepaliveInterval` from the kernel via wgctrl but **explicitly discards it**:

```go
// internal/wg/device.go:163
_ = p.PersistentKeepaliveInterval
```

This means:
1. Users set keepalive when creating a peer, but can't see the current kernel value
2. No way to verify if keepalive is actually applied to the WireGuard interface
3. Reconciliation can't detect keepalive mismatches between DB and kernel
4. The peer detail view is incomplete — it shows everything except keepalive

This is a simple fix: stop discarding the value and expose it in the API and UI.

## User Stories

- **NAT-traversal user**: "I set keepalive to 25 seconds. Is it actually applied? The UI doesn't show it."
- **Debugger**: "A peer behind NAT keeps disconnecting. Is keepalive set correctly on the server side?"

## Design

### No Data Model Changes

The `persistent_keepalive` column already exists in the peers table and is already used during peer configuration. This feature only adds the **read-back** path.

### Code Changes

**1. Add field to WGPeerInfo** (`internal/wg/device.go`):

Current struct:
```go
type WGPeerInfo struct {
    PublicKey     string
    PresharedKey  string
    Endpoint      string
    AllowedIPs    []string
    LastHandshake time.Time
    ReceiveBytes  int64
    TransmitBytes int64
}
```

Add:
```go
type WGPeerInfo struct {
    // ... existing fields ...
    PersistentKeepalive time.Duration
}
```

**2. Stop discarding the value** (`internal/wg/device.go:163`):

Change:
```go
_ = p.PersistentKeepaliveInterval
```

To:
```go
info.PersistentKeepalive = p.PersistentKeepaliveInterval
```

**3. Add to API response** — peer status and peer detail endpoints should include `persistent_keepalive_seconds`:

```json
{
  "public_key": "abc123...",
  "endpoint": "1.2.3.4:51820",
  "persistent_keepalive_seconds": 25,
  "last_handshake": "2024-01-15T10:30:00Z",
  "receive_bytes": 123456,
  "transmit_bytes": 654321
}
```

**4. Add to reconciliation** — compare DB value vs kernel value:

```go
if dbPeer.PersistentKeepalive != kernelPeer.PersistentKeepalive {
    log.Warn("keepalive mismatch",
        "peer", dbPeer.PublicKey[:8],
        "db", dbPeer.PersistentKeepalive,
        "kernel", kernelPeer.PersistentKeepalive,
    )
    // Correct kernel to match DB
}
```

### Frontend Changes

- **Peer table**: Add "Keepalive" column showing value in seconds or "Off"
- **Peer detail**: Show keepalive interval with explanation text
- **Peer form**: Already has keepalive input — no changes needed

### Prometheus Metrics

Optional: add per-peer keepalive gauge:

```
wg_peer_persistent_keepalive_seconds{network="office",peer="laptop"} 25
```

## Implementation Steps

1. Add `PersistentKeepalive` field to `WGPeerInfo` struct
2. Assign the value in `fromWGPeer` instead of discarding
3. Include in peer API response
4. Add to reconciliation comparison
5. Add keepalive column to peer table in frontend
6. Write tests verifying the value round-trips correctly

## Validation

- Peer created with keepalive=25 shows `persistent_keepalive_seconds: 25` in API response
- Peer created with keepalive=0 shows `persistent_keepalive_seconds: 0` (disabled)
- Reconciliation detects mismatch between DB and kernel keepalive values
- Frontend displays keepalive column correctly

## Cross-References

- [peer-management.md](peer-management.md) — Peer model and config generation
- [monitoring.md](monitoring.md) — Peer status includes keepalive info
- [feat-010-device-type-diagnostics.md](feat-010-device-type-diagnostics.md) — Diagnostics could warn about missing keepalive for peers behind NAT
- [architecture/api-surface.md](../architecture/api-surface.md) — Extended peer response
