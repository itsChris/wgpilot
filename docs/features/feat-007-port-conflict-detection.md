# feat-007: Port Conflict Detection (Socket Diagnostics)

> **Status:** Proposed
> **Priority:** Tier 2 — Medium Impact
> **Effort:** Low
> **Library:** vishvananda/netlink (`SocketDiagUDP`, `SocketDiagTCPInfo`)
> **Unique:** Mostly — provides clear error messages instead of cryptic netlink failures

---

## Motivation

When wgpilot tries to create a WireGuard interface on a port that's already in use, the kernel returns a cryptic netlink error: `"address already in use"`. The user has no idea what process is using the port or how to fix it.

Linux socket diagnostics (the `ss` subsystem) can enumerate all open sockets on the system — UDP and TCP — with their owning process. By querying this before creating an interface, wgpilot can give a clear, actionable error: "Port 51820 is already in use by wireguard-go (pid 1234)."

This also benefits:
- The `wgpilot diagnose` CLI command (port conflict check)
- Network creation validation (pre-check before attempting)
- System info endpoint (show all listening ports)

## User Stories

- **New user**: "I get 'address already in use' when creating a network. What does that mean?"
- **Migrating user**: "I'm importing an existing WireGuard setup but the port is still bound by the old wg-quick service."
- **Multi-network admin**: "I accidentally tried to use the same port for two networks."

## Design

### No Data Model Changes

Socket diagnostics are read-only kernel queries. No database changes needed.

### API Changes

**Enhanced validation on existing endpoints:**

`POST /api/networks` — before creating the interface, check if the port is available. Return a clear error if not:

```json
{
  "error": "port_in_use",
  "message": "UDP port 51820 is already in use",
  "details": {
    "port": 51820,
    "protocol": "udp",
    "process": "wireguard-go",
    "pid": 1234,
    "local_address": "0.0.0.0:51820"
  }
}
```

**New diagnostic endpoint:**

`GET /api/system/ports` — list relevant listening ports:

```json
{
  "wireguard_ports": [
    {
      "port": 51820,
      "interface": "wg0",
      "managed": true
    }
  ],
  "conflicting_ports": [],
  "https_port": {
    "port": 443,
    "pid": 5678,
    "process": "wgpilot"
  }
}
```

### Kernel Implementation

```go
type PortChecker struct {
    logger *slog.Logger
}

// CheckUDPPort returns nil if the port is available, or a descriptive error if in use.
func (c *PortChecker) CheckUDPPort(port uint16) error {
    sockets, err := netlink.SocketDiagUDP(unix.AF_INET)
    if err != nil {
        return fmt.Errorf("query UDP sockets: %w", err)
    }

    for _, s := range sockets {
        if s.ID.SourcePort == port {
            return &PortInUseError{
                Port:     port,
                Protocol: "udp",
                PID:      s.ID.Cookie[0], // process info may need /proc lookup
                LocalAddr: s.ID.Source.String(),
            }
        }
    }

    // Also check IPv6
    sockets6, err := netlink.SocketDiagUDP(unix.AF_INET6)
    if err == nil {
        for _, s := range sockets6 {
            if s.ID.SourcePort == port {
                return &PortInUseError{
                    Port:     port,
                    Protocol: "udp6",
                    LocalAddr: s.ID.Source.String(),
                }
            }
        }
    }

    return nil
}

// ListUDPListeners returns all UDP sockets for diagnostic purposes.
func (c *PortChecker) ListUDPListeners() ([]ListeningSocket, error) {
    sockets, err := netlink.SocketDiagUDP(unix.AF_INET)
    if err != nil {
        return nil, fmt.Errorf("query UDP sockets: %w", err)
    }

    var result []ListeningSocket
    for _, s := range sockets {
        result = append(result, ListeningSocket{
            Protocol:  "udp",
            LocalAddr: s.ID.Source.String(),
            Port:      s.ID.SourcePort,
        })
    }
    return result, nil
}

type PortInUseError struct {
    Port      uint16
    Protocol  string
    PID       uint32
    Process   string
    LocalAddr string
}

func (e *PortInUseError) Error() string {
    if e.Process != "" {
        return fmt.Sprintf("UDP port %d is already in use by %s (pid %d)",
            e.Port, e.Process, e.PID)
    }
    return fmt.Sprintf("UDP port %d is already in use (bound to %s)",
        e.Port, e.LocalAddr)
}
```

**Process name resolution** (optional, requires /proc access):

```go
func processNameFromPID(pid uint32) string {
    data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(data))
}
```

### Integration Points

**Network creation validation** (`internal/server/routes_networks.go`):

```go
func (s *Server) handleCreateNetwork(w http.ResponseWriter, r *http.Request) {
    // ... parse request ...

    // Pre-check port availability
    if err := s.portChecker.CheckUDPPort(uint16(req.ListenPort)); err != nil {
        var portErr *PortInUseError
        if errors.As(err, &portErr) {
            writeError(w, r, portErr, http.StatusConflict, s.devMode)
            return
        }
    }

    // ... proceed with creation ...
}
```

**Diagnose CLI** (`cmd/wgpilot/diagnose.go`):

Add a "Port Conflicts" check that scans all configured WireGuard ports:

```
[PASS] Port 51820 (wg0): available
[FAIL] Port 51821 (wg1): in use by wireguard-go (pid 1234)
```

### Package Layout

```
internal/portcheck/
├── checker.go      — PortChecker with CheckUDPPort, ListUDPListeners
├── types.go        — PortInUseError, ListeningSocket
└── checker_test.go — Tests
```

### Frontend Changes

- **Network form**: Real-time port availability check (debounced API call as user types port number)
- **System info page**: Show listening ports table
- **Error display**: When port conflict occurs, show the specific process and PID instead of generic error

## Implementation Steps

1. Implement `internal/portcheck/checker.go` with `CheckUDPPort` and `ListUDPListeners`
2. Wire into network creation handler as pre-validation
3. Wire into network update handler (port change)
4. Add port check to `wgpilot diagnose`
5. Add `GET /api/system/ports` endpoint
6. Add real-time port validation to network form in frontend
7. Write tests

## Validation

- Creating a network on an occupied port returns 409 with process details
- Creating a network on a free port succeeds normally
- `wgpilot diagnose` reports port conflicts clearly
- `GET /api/system/ports` lists all relevant ports
- IPv4 and IPv6 sockets are both checked
- Graceful degradation if socket diagnostics fail (fall back to attempting bind)

## Cross-References

- [network-management.md](network-management.md) — Port validation during network creation
- [feat-006-active-connection-viewer.md](feat-006-active-connection-viewer.md) — Complementary debugging tool
- [feat-010-device-type-diagnostics.md](feat-010-device-type-diagnostics.md) — Diagnostics integration
- [operations/service.md](../operations/service.md) — Diagnose CLI enhancements
- [architecture/api-surface.md](../architecture/api-surface.md) — New endpoint
