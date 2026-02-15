# feat-010: WireGuard Device Type in Diagnostics

> **Status:** Proposed
> **Priority:** Tier 3 — Quick Win
> **Effort:** Very Low
> **Library:** wgctrl-go (`Device.Type` — currently not captured)
> **Unique:** No, but helps users identify performance issues

---

## Motivation

wgctrl-go reports whether each WireGuard device is running as a **kernel module** or in **userspace** (wireguard-go). This information is available via `Device.Type` but wgpilot currently ignores it.

This matters because:
- **Kernel WireGuard** is 3-4x faster than userspace wireguard-go
- Users may unknowingly be running userspace WireGuard (e.g., old kernels, containers without module access)
- The `wgpilot diagnose` command should flag userspace as a performance warning
- The system info endpoint should report which backend each interface uses

## User Stories

- **Performance troubleshooter**: "My WireGuard throughput is much lower than expected. Am I running kernel or userspace?"
- **Docker user**: "I'm running wgpilot in Docker but getting low performance. Is the WireGuard kernel module loaded?"
- **Old kernel user**: "I'm on Ubuntu 18.04. Is WireGuard running natively or via wireguard-go?"

## Design

### No Data Model Changes

Device type is read live from the kernel. No database changes needed.

### Code Changes

**1. Capture Device.Type** (`internal/wg/device.go`):

Current `DeviceInfo` struct:
```go
type DeviceInfo struct {
    Name       string
    PublicKey  string
    ListenPort int
    Peers      []WGPeerInfo
}
```

Add:
```go
type DeviceInfo struct {
    Name       string
    PublicKey  string
    ListenPort int
    DeviceType string // "Linux kernel", "userspace", "unknown"
    Peers      []WGPeerInfo
}
```

In `fromWGDevice`:
```go
func fromWGDevice(d *wgtypes.Device) DeviceInfo {
    return DeviceInfo{
        Name:       d.Name,
        PublicKey:  d.PublicKey.String(),
        ListenPort: d.ListenPort,
        DeviceType: d.Type.String(), // "Linux kernel", "userspace", etc.
        Peers:     peers,
    }
}
```

**2. Expose in API responses:**

`GET /api/status`:
```json
{
  "networks": [
    {
      "id": 1,
      "interface": "wg0",
      "device_type": "Linux kernel",
      "peers": [...]
    }
  ]
}
```

`GET /api/system/info`:
```json
{
  "version": "0.2.0",
  "wireguard_interfaces": [
    {
      "name": "wg0",
      "device_type": "Linux kernel"
    }
  ],
  "wireguard_kernel_module": true
}
```

**3. Diagnose CLI check:**

```
=== WireGuard Backend ===
[PASS] wg0: Linux kernel
[WARN] wg1: userspace (wireguard-go) — kernel module provides 3-4x better performance
[INFO] Kernel module loaded: wireguard (version 1.0.0)
```

**4. Kernel module detection** (for diagnostics when no interfaces exist yet):

```go
func isKernelModuleLoaded() bool {
    data, err := os.ReadFile("/proc/modules")
    if err != nil {
        return false
    }
    return strings.Contains(string(data), "wireguard")
}
```

### Package Changes

Minimal changes — all within existing files:
- `internal/wg/device.go` — add field, assign value
- `internal/server/routes_status.go` — include in response
- `internal/debug/` — add to diagnose checks

### Frontend Changes

- **Network card/detail**: Show device type badge:
  - "Kernel" badge (green) for Linux kernel
  - "Userspace" badge (yellow/warning) for wireguard-go
- **System info page**: Show WireGuard backend per interface
- **Dashboard**: Warning banner if any interface is running in userspace mode

### Alert Integration (Optional)

Add an alert type: `wireguard_userspace` — triggers when a WireGuard interface is detected running in userspace mode. This is a one-time informational alert, not recurring.

## Implementation Steps

1. Add `DeviceType` field to `DeviceInfo` struct
2. Assign `d.Type.String()` in `fromWGDevice`
3. Include `device_type` in status API response
4. Include in system info endpoint
5. Add kernel module check to `wgpilot diagnose`
6. Add device type badge to network cards in frontend
7. Write tests

## Validation

- `GET /api/status` includes `device_type` field for each network
- Kernel WireGuard returns `"Linux kernel"`
- Userspace wireguard-go returns `"userspace"`
- `wgpilot diagnose` flags userspace as a performance warning
- `/proc/modules` check detects kernel module presence
- Graceful handling when device type is "unknown"

## Cross-References

- [monitoring.md](monitoring.md) — Status endpoint extended with device type
- [feat-003-interface-level-statistics.md](feat-003-interface-level-statistics.md) — Interface stats context (kernel vs userspace affects interpretation)
- [feat-005-mtu-management.md](feat-005-mtu-management.md) — MTU behavior may differ between kernel and userspace
- [feat-007-port-conflict-detection.md](feat-007-port-conflict-detection.md) — Combined diagnostics
- [operations/service.md](../operations/service.md) — Diagnose CLI enhancements
- [architecture/api-surface.md](../architecture/api-surface.md) — Extended response fields
