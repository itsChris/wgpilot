# Monitoring & Observability

> **Purpose**: Specifies the dashboard UI, live peer status via SSE, structured logging, Prometheus metrics, health endpoint, historical snapshots, retention policy, and alert system.
>
> **Related docs**: [../architecture/data-model.md](../architecture/data-model.md), [../architecture/api-surface.md](../architecture/api-surface.md)
>
> **Implements**: `internal/monitor/`, `internal/server/handlers/status.go`, `frontend/src/components/dashboard/`

---

## Layer 1: Dashboard (Built into UI)

Real-time overview on the main page after login:

- **Stats cards:** Total networks, total peers, online peers, total transfer.
- **Per-network peer table:** Name, status dot (green/gray), last handshake, transfer RX/TX.
- **Transfer chart:** 24-hour transfer graph per network (Recharts).

Data source: kernel via wgctrl (polled every 30 seconds), delivered to UI via SSE.

### Live Updates via SSE

The dashboard subscribes to server-sent events for real-time peer status:

```
GET /api/networks/:id/events
Content-Type: text/event-stream

event: status
data: [{"peer_id":1,"online":true,"last_handshake":1739...,"transfer_rx":...}]

event: status
data: [...]
```

Go backend SSE handler:

```go
func (h *Handler) SSEStatus(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    flusher := w.(http.Flusher)

    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-r.Context().Done():
            return
        case <-ticker.C:
            status, _ := h.manager.PeerStatus(networkID)
            data, _ := json.Marshal(status)
            fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
            flusher.Flush()
        }
    }
}
```

Frontend SSE hook updates TanStack Query cache directly, avoiding refetches:

```typescript
useSSE(`/networks/${networkId}/events`, (peers: PeerStatus[]) => {
    queryClient.setQueryData(['networks', networkId, 'peers'], (old) =>
        old?.map(p => {
            const update = peers.find(s => s.peer_id === p.id);
            return update ? { ...p, ...update } : p;
        })
    );
});
```

### TypeScript Types

```typescript
interface PeerStatus {
    peer_id: number;
    online: boolean;
    last_handshake: number;
    transfer_rx: number;
    transfer_tx: number;
}
```

## Layer 2: Structured Logging

JSON logs to stdout, captured by journald:

```json
{"time":"...","level":"INFO","msg":"peer_handshake","network":"wg0","peer":"My Phone","endpoint":"98.42.1.100:34821"}
{"time":"...","level":"INFO","msg":"peer_added","network":"wg0","peer":"New Laptop","admin":"admin"}
{"time":"...","level":"WARN","msg":"peer_offline","network":"wg0","peer":"Dad's PC","last_seen":"72h"}
{"time":"...","level":"INFO","msg":"http_request","method":"POST","path":"/api/networks/1/peers","status":201,"duration_ms":12,"user":"admin"}
```

Log levels: DEBUG, INFO, WARN, ERROR. Configurable via `config.yaml` and reloadable via SIGHUP.

Uses `log/slog` from the standard library:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: configuredLevel,
}))
```

What gets logged:
- Every admin action (add/remove/edit peer, change network config) → audit entry
- Every state change (peer online/offline, interface up/down)
- HTTP requests → standard access log fields

Users query with standard tools:

```bash
journalctl -u wg-webui -f                          # tail live
journalctl -u wg-webui --since "1 hour ago" -o json # structured query
journalctl -u wg-webui | grep peer_offline           # find issues
```

## Layer 3: Health & Metrics

### Health Endpoint (unauthenticated)

```
GET /health → 200
{
    "status": "healthy",
    "version": "1.2.0",
    "uptime": "14d 3h 22m",
    "networks": { "total": 2, "healthy": 2, "degraded": 0 },
    "database": "ok"
}
```

### Prometheus Metrics (unauthenticated, optionally gated)

```
GET /metrics

wg_peers_total{network="wg0"} 9
wg_peers_online{network="wg0"} 7
wg_transfer_bytes_total{network="wg0",direction="rx"} 574893021
wg_peer_last_handshake_seconds{network="wg0",peer="My Phone"} 45
wg_interface_up{network="wg0"} 1
wg_webui_http_requests_total{method="GET",status="200"} 14832
```

## Historical Data

The monitoring poller writes snapshots to SQLite every 30 seconds. See [../architecture/data-model.md](../architecture/data-model.md) for the `peer_snapshots` table schema.

### Retention Policy

| Age | Granularity |
|---|---|
| < 24 hours | 30-second raw data |
| 1-30 days | Hourly aggregates |
| 30-365 days | Daily aggregates |
| > 1 year | Deleted |

Background compaction job runs daily.

## Alerts

Simple alert rules stored in SQLite (see [../architecture/data-model.md](../architecture/data-model.md) for the `alerts` table schema), configured via UI:

| Type | Threshold | Action |
|---|---|---|
| `peer_offline` | Duration (e.g., 10m) | Email |
| `interface_down` | Immediate | Email |
| `transfer_spike` | Rate (e.g., 1GB/hour) | Email |

Email via SMTP configured in settings. No webhook/Slack/PagerDuty — users can point those tools at `/metrics` or parse JSON logs.

---

## v0.3.0 Enhancements (Proposed)

The following proposed features extend the monitoring system:

- [feat-003: Interface-Level Statistics](feat-003-interface-level-statistics.md) — Add RX/TX/errors/drops counters at the interface level (via `netlink.Link.Statistics`), plus new Prometheus metrics
- [feat-004: Event-Driven Monitoring](feat-004-event-driven-monitoring.md) — Replace/augment 30s polling with instant netlink subscriptions for link/route/address changes
- [feat-006: Active Connection Viewer](feat-006-active-connection-viewer.md) — View active TCP/UDP connections through WireGuard tunnels via conntrack
- [feat-008: PersistentKeepalive Display](feat-008-persistent-keepalive-display.md) — Expose the currently-discarded keepalive interval in peer status
- [feat-010: Device Type Diagnostics](feat-010-device-type-diagnostics.md) — Show kernel vs userspace WireGuard backend in status API
