# API Surface

> **Purpose**: Defines all REST endpoints, request/response shapes, status codes, wire formats, and conventions.
>
> **Related docs**: [data-model.md](data-model.md), [../features/auth.md](../features/auth.md), [../features/monitoring.md](../features/monitoring.md)
>
> **Implements**: `internal/server/router.go`, `internal/server/handlers/`

---

All endpoints are prefixed with `/api`. Authentication is required for all endpoints except `/api/auth/login` and `/api/setup/*` (during first-run only).

## Authentication

```
POST   /api/auth/login              # username/password → JWT
POST   /api/auth/logout             # invalidate session
GET    /api/auth/me                 # current user info
PUT    /api/auth/password           # change password
```

## Setup (first-run only, disabled after setup_complete=true)

```
POST   /api/setup/admin             # Step 1: create admin account
PUT    /api/setup/server            # Step 2: confirm server identity
POST   /api/setup/network           # Step 3: create first network
POST   /api/setup/peer              # Step 4: create first peer
GET    /api/setup/status            # which steps are complete
POST   /api/setup/import            # import existing WG interfaces
```

## Networks

```
GET    /api/networks                # list all networks
POST   /api/networks                # create network
GET    /api/networks/:id            # get network details
PUT    /api/networks/:id            # update network
DELETE /api/networks/:id            # delete network (tears down interface)
POST   /api/networks/:id/enable     # enable network (bring up interface)
POST   /api/networks/:id/disable    # disable network (bring down interface)
```

## Peers

```
GET    /api/networks/:id/peers              # list peers in network
POST   /api/networks/:id/peers              # create peer
GET    /api/networks/:id/peers/:pid         # get peer details
PUT    /api/networks/:id/peers/:pid         # update peer
DELETE /api/networks/:id/peers/:pid         # delete peer
POST   /api/networks/:id/peers/:pid/enable  # enable peer
POST   /api/networks/:id/peers/:pid/disable # disable peer
GET    /api/networks/:id/peers/:pid/config  # download .conf file
GET    /api/networks/:id/peers/:pid/qr      # get QR code (PNG)
```

## Network Bridges

```
GET    /api/bridges                 # list all bridges
POST   /api/bridges                 # create bridge between networks
GET    /api/bridges/:id             # get bridge details
PUT    /api/bridges/:id             # update bridge
DELETE /api/bridges/:id             # delete bridge
```

## Status & Monitoring

```
GET    /api/status                  # live interface stats from kernel
GET    /api/networks/:id/events     # SSE stream for real-time peer status
GET    /api/networks/:id/stats      # historical transfer data (query params: from, to, granularity)
```

## Settings

```
GET    /api/settings                # get all settings (sensitive values redacted)
PUT    /api/settings                # update settings
GET    /api/settings/tls            # TLS status (cert expiry, mode)
POST   /api/settings/tls/test       # test ACME provisioning
```

## Alerts

```
GET    /api/alerts                  # list alert rules
POST   /api/alerts                  # create alert rule
PUT    /api/alerts/:id              # update alert rule
DELETE /api/alerts/:id              # delete alert rule
```

## System

```
GET    /health                      # health check (no auth required)
GET    /metrics                     # Prometheus metrics (no auth required, optionally gated)
GET    /api/system/info             # version, uptime, OS info
POST   /api/system/backup           # trigger database backup (returns file)
POST   /api/system/restore          # restore from backup upload
GET    /api/audit-log               # query audit log (query params: from, to, action, limit, offset)
```

## Request/Response Conventions

- All request/response bodies are JSON (`Content-Type: application/json`).
- Timestamps are unix epoch integers.
- List endpoints support `?limit=N&offset=M` for pagination.
- Error responses follow a consistent format:

```json
{
    "error": {
        "code": "PEER_NOT_FOUND",
        "message": "peer 42 does not exist in network 1"
    }
}
```

- HTTP status codes: 200 (OK), 201 (Created), 204 (No Content for deletes), 400 (Bad Request), 401 (Unauthorized), 404 (Not Found), 409 (Conflict), 500 (Internal Server Error).

## Wire Formats

### Create Network Request

```json
POST /api/networks
{
    "name": "Home VPN",
    "mode": "gateway",
    "subnet": "10.0.0.0/24",
    "listen_port": 51820,
    "dns_servers": "1.1.1.1,8.8.8.8",
    "nat_enabled": true,
    "inter_peer_routing": false
}
```

### Create Network Response

```json
201 Created
{
    "id": 1,
    "name": "Home VPN",
    "interface": "wg0",
    "mode": "gateway",
    "subnet": "10.0.0.0/24",
    "listen_port": 51820,
    "public_key": "abc123...",
    "dns_servers": "1.1.1.1,8.8.8.8",
    "nat_enabled": true,
    "inter_peer_routing": false,
    "enabled": true,
    "created_at": 1739000000,
    "updated_at": 1739000000
}
```

### Create Peer Request

```json
POST /api/networks/1/peers
{
    "name": "My Phone",
    "email": "chris@example.com",
    "role": "client",
    "persistent_keepalive": 25
}
```

### Create Peer Response

```json
201 Created
{
    "id": 1,
    "network_id": 1,
    "name": "My Phone",
    "email": "chris@example.com",
    "public_key": "xyz789...",
    "allowed_ips": "10.0.0.2/32",
    "endpoint": "",
    "persistent_keepalive": 25,
    "role": "client",
    "site_networks": "",
    "enabled": true,
    "created_at": 1739000000,
    "updated_at": 1739000000
}
```

### Peer Config (.conf)

```ini
[Interface]
PrivateKey = <client-private-key>
Address = 10.0.0.2/32
DNS = 1.1.1.1,8.8.8.8

[Peer]
PublicKey = <server-public-key>
PresharedKey = <preshared-key>
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.45:51820
PersistentKeepalive = 25
```

### Status Response

```json
GET /api/status
{
    "networks": [
        {
            "id": 1,
            "name": "Home VPN",
            "interface": "wg0",
            "enabled": true,
            "up": true,
            "listen_port": 51820,
            "peers": [
                {
                    "id": 1,
                    "name": "My Phone",
                    "public_key": "xyz789...",
                    "endpoint": "98.42.1.100:34821",
                    "last_handshake": 1739000045,
                    "transfer_rx": 574893021,
                    "transfer_tx": 335102847,
                    "online": true
                }
            ]
        }
    ]
}
```
