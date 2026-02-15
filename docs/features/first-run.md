# First-Run Experience

> **Purpose**: Specifies the setup wizard flow, its 4 steps, backend actions per step, edge cases, and existing WireGuard import.
>
> **Related docs**: [auth.md](auth.md), [network-management.md](network-management.md), [peer-management.md](peer-management.md), [../architecture/api-surface.md](../architecture/api-surface.md)
>
> **Implements**: `internal/server/handlers/setup.go`, `frontend/src/components/setup/`

---

## Flow

```
Install Script Finishes
    │
    ▼
Print: URL + one-time password
    │
    ▼
User opens URL in browser
    │
    ▼
App detects setup_complete=false
    │
    ▼
Redirect to /setup
    │
    ▼
Step 1: Create Admin Account
    ├── Authenticate with install token
    ├── Choose username + password
    ├── INSERT user, DELETE install token
    └── Issue JWT session
    │
    ▼
Step 2: Server Identity
    ├── Confirm/edit auto-detected public IP
    ├── Optional: enter hostname → triggers ACME
    ├── Set default DNS servers
    └── UPDATE settings
    │
    ▼
Step 3: First Network
    ├── Name, mode, subnet, listen port
    ├── NAT toggle
    ├── Generate server keypair
    ├── Create interface, assign IP, apply rules
    └── INSERT network
    │
    ▼
Step 4: First Peer
    ├── Name, type (client/site-gateway)
    ├── Full/split tunnel toggle
    ├── Generate client keypair
    ├── Add peer to kernel
    ├── INSERT peer
    ├── Show QR code + .conf download
    └── SET setup_complete=true
    │
    ▼
Redirect to Dashboard
```

## Backend Actions Per Step

```
Step 1:  INSERT admin user (bcrypt hash)
         DELETE one-time install token
         Issue JWT session

Step 2:  UPDATE settings (public_ip, hostname, dns)
         IF hostname → trigger ACME cert provision
         Restart HTTPS listener with new cert

Step 3:  INSERT network record
         GeneratePrivateKey()
         netlink.LinkAdd(wg0)
         netlink.AddrAdd(10.0.0.1/24)
         wgctrl.ConfigureDevice(key, port)
         netlink.LinkSetUp(wg0)
         nftables: add MASQUERADE rule (if NAT)
         sysctl: ensure ip_forward=1

Step 4:  INSERT peer record
         GeneratePrivateKey() for client
         wgctrl.ConfigureDevice(add peer)
         Generate .conf from template
         Encode .conf as QR
         Mark setup_complete=true in settings
```

## Edge Cases

- **Browser closed mid-wizard:** Steps are idempotent. App tracks which steps are complete and resumes where the user left off. On next visit, the app checks `setup_complete` in the database. If false, redirect back to the wizard at whichever step is incomplete.

- **Existing WireGuard interfaces detected:** Before Step 3, show an import prompt. Import reads kernel state via wgctrl and parses existing `/etc/wireguard/*.conf` files for metadata (peer names, DNS settings). Populates the database from the discovered state.

- **Port 443 occupied:** Fall back to 8443. Config file allows changing permanently.

- **ACME fails:** Fall back to self-signed, show warning in UI. Not a blocker for setup.

## Setup API Endpoints

See [../architecture/api-surface.md](../architecture/api-surface.md) for the full setup endpoint definitions:

```
POST   /api/setup/admin             # Step 1
PUT    /api/setup/server            # Step 2
POST   /api/setup/network           # Step 3
POST   /api/setup/peer              # Step 4
GET    /api/setup/status            # which steps are complete
POST   /api/setup/import            # import existing WG interfaces
```

All setup endpoints are disabled after `setup_complete=true`.
