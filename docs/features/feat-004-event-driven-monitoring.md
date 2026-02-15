# feat-004: Event-Driven Monitoring

> **Status:** Proposed
> **Priority:** Tier 2 — Medium Impact
> **Effort:** Medium
> **Library:** vishvananda/netlink (`LinkSubscribe`, `RouteSubscribe`, `AddrSubscribe`)
> **Unique:** No, but significantly improves responsiveness

---

## Motivation

wgpilot's monitor currently polls WireGuard peer status every 30 seconds. This means:

1. An interface going down is detected up to 30 seconds late
2. External changes to routes or IP addresses go unnoticed until the next poll
3. Polling consumes CPU cycles even when nothing changes
4. SSE clients receive stale data between poll intervals

Linux netlink supports **subscription-based notifications** — the kernel pushes events to a Go channel whenever link state, routes, or addresses change. This is how tools like `ip monitor` work, and it's available through vishvananda/netlink.

Combining event-driven detection with the existing poll cycle gives the best of both worlds: instant reaction to kernel events, with periodic reconciliation as a safety net.

## User Stories

- **Dashboard viewer**: "I want to see a peer go offline the moment it happens, not 30 seconds later."
- **Alert system**: "Trigger the 'interface down' alert immediately, not on the next poll."
- **Sysadmin**: "If someone manually changes a route on the box, wgpilot should detect it and warn me."

## Design

### No Data Model Changes

This feature changes how monitoring works internally but doesn't alter the database schema.

### Architecture

```
                        ┌─────────────────────┐
                        │   Linux Kernel       │
                        │                      │
                        │  Link events         │
                        │  Route events        │
                        │  Address events      │
                        └──────┬──┬──┬─────────┘
                               │  │  │
                     netlink subscriptions
                               │  │  │
                        ┌──────▼──▼──▼─────────┐
                        │  EventWatcher         │
                        │                       │
                        │  Filters events by    │
                        │  WireGuard interfaces │
                        │                       │
                        │  Pushes to channels:  │
                        │  - linkChanges        │
                        │  - routeChanges       │
                        │  - addrChanges        │
                        └──────────┬────────────┘
                                   │
                        ┌──────────▼────────────┐
                        │  Monitor (enhanced)    │
                        │                        │
                        │  Reacts to events:     │
                        │  - Trigger poll early  │
                        │  - Log external change │
                        │  - Push SSE update     │
                        │  - Evaluate alerts     │
                        │                        │
                        │  Still polls every 30s │
                        │  as reconciliation     │
                        └───────────────────────┘
```

### Kernel Implementation

```go
type EventWatcher struct {
    linkCh   chan netlink.LinkUpdate
    routeCh  chan netlink.RouteUpdate
    addrCh   chan netlink.AddrUpdate
    done     chan struct{}
    wgIfaces map[string]bool // tracked WireGuard interfaces
    handler  EventHandler
}

type EventHandler interface {
    OnLinkChange(ifaceName string, up bool)
    OnRouteChange(ifaceName string, route netlink.Route, added bool)
    OnAddrChange(ifaceName string, addr netlink.Addr, added bool)
}

func (w *EventWatcher) Start(ctx context.Context) error {
    // Subscribe to link events
    if err := netlink.LinkSubscribeWithOptions(w.linkCh, w.done,
        netlink.LinkSubscribeOptions{ErrorCallback: w.onError}); err != nil {
        return fmt.Errorf("subscribe link events: %w", err)
    }

    // Subscribe to route events
    if err := netlink.RouteSubscribeWithOptions(w.routeCh, w.done,
        netlink.RouteSubscribeOptions{ErrorCallback: w.onError}); err != nil {
        return fmt.Errorf("subscribe route events: %w", err)
    }

    // Subscribe to address events
    if err := netlink.AddrSubscribeWithOptions(w.addrCh, w.done,
        netlink.AddrSubscribeOptions{ErrorCallback: w.onError}); err != nil {
        return fmt.Errorf("subscribe addr events: %w", err)
    }

    go w.eventLoop(ctx)
    return nil
}

func (w *EventWatcher) eventLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            close(w.done)
            return
        case update := <-w.linkCh:
            name := update.Link.Attrs().Name
            if w.wgIfaces[name] {
                up := update.Link.Attrs().Flags&net.FlagUp != 0
                w.handler.OnLinkChange(name, up)
            }
        case update := <-w.routeCh:
            // Filter for routes involving WireGuard interfaces
            // ...
        case update := <-w.addrCh:
            // Filter for address changes on WireGuard interfaces
            // ...
        }
    }
}
```

### Integration with Existing Monitor

The existing `Poller` implements `EventHandler`:

```go
func (p *Poller) OnLinkChange(ifaceName string, up bool) {
    p.logger.Info("interface state changed",
        "interface", ifaceName,
        "up", up,
        "source", "kernel_event",
    )
    // Trigger immediate poll for this interface
    p.triggerPoll(ifaceName)
    // Evaluate alerts immediately
    p.evaluateAlerts(ifaceName)
}
```

### Event Types to Track

| Event | Source | Action |
|-------|--------|--------|
| Link up/down | `LinkSubscribe` | Immediate SSE push, alert evaluation, audit log |
| Link deleted | `LinkSubscribe` | Log warning (external deletion of managed interface) |
| Route added/removed | `RouteSubscribe` | Log if unexpected, reconcile if managed route was removed |
| Address changed | `AddrSubscribe` | Log warning, reconcile if managed address was changed |

### Package Changes

```
internal/monitor/
├── watcher.go       — EventWatcher with netlink subscriptions
├── watcher_test.go  — Tests with mocked netlink channels
└── poller.go        — Enhanced with EventHandler interface
```

### Frontend Changes

No direct frontend changes needed. SSE updates will be pushed faster, making the dashboard feel more responsive.

### Configuration

```yaml
monitor:
  poll_interval: "30s"           # Existing: periodic reconciliation
  event_driven: true             # New: enable netlink subscriptions (default: true)
```

## Implementation Steps

1. Define `EventHandler` interface in `internal/monitor/`
2. Implement `EventWatcher` with `LinkSubscribe`, `RouteSubscribe`, `AddrSubscribe`
3. Add WireGuard interface filtering (only track managed interfaces)
4. Implement `EventHandler` on existing `Poller`
5. Wire `EventWatcher` into `main.go` startup/shutdown
6. Add `event_driven` config option
7. Write tests with mocked channels

## Validation

- When a WireGuard interface goes down (`ip link set wg0 down`), SSE clients receive update within 1 second
- When a managed route is removed externally, wgpilot logs a warning and restores it
- When a managed address is changed externally, wgpilot logs and reconciles
- Non-WireGuard interface events are filtered out (no noise)
- Event watcher gracefully stops on context cancellation
- Polling continues as a fallback even with event watcher enabled

## Cross-References

- [monitoring.md](monitoring.md) — Core monitoring system that this enhances
- [feat-002-split-tunnel-policy-routing.md](feat-002-split-tunnel-policy-routing.md) — Route events enable detection of external route changes
- [feat-003-interface-level-statistics.md](feat-003-interface-level-statistics.md) — Link events trigger stat collection
- [feat-009-route-table-viewer.md](feat-009-route-table-viewer.md) — Route events update the route viewer in real-time
