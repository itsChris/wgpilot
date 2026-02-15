# TLS & Certificate Management

> **Purpose**: Specifies the three HTTPS modes (self-signed, ACME/Let's Encrypt, manual), certificate provisioning, fallback behavior, and integration with the setup wizard.
>
> **Related docs**: [service.md](service.md), [../features/first-run.md](../features/first-run.md)
>
> **Implements**: `internal/tls/`

---

## TLS Modes

The Go binary serves HTTPS directly — no nginx/Caddy dependency. Three modes:

### Self-Signed (default)

- Generated on first boot if no cert exists.
- Stored in `/var/lib/wg-webui/certs/`.
- Used when no hostname is configured (IP-only access).
- Browser will show a certificate warning.

### ACME (Let's Encrypt)

- Activated when a hostname is set (during setup wizard Step 2 or via settings).
- Uses `golang.org/x/crypto/acme/autocert` for automatic provisioning and renewal.
- Requires port 443 to be reachable from the internet for the ACME challenge.
- Certificates stored in `/var/lib/wg-webui/certs/`.
- HTTP → HTTPS redirect on port 80 (optional, for ACME challenges).

### Manual

- User provides their own cert and key files via `config.yaml`:

```yaml
tls:
  mode: manual
  cert_file: /path/to/cert.pem
  key_file: /path/to/key.pem
```

## Fallback Behavior

- If ACME provisioning fails (port blocked, DNS not pointed, rate limited), the app falls back to self-signed and logs a warning.
- This is not a blocker for setup — the wizard continues with self-signed.
- The UI shows TLS status on the settings page (cert expiry, current mode).

## Configuration

TLS settings in `config.yaml` (see [service.md](service.md) for the full config file):

```yaml
tls:
  mode: self-signed    # self-signed | acme | manual
  domain: ""           # required for acme mode
  cert_file: ""        # required for manual mode
  key_file: ""         # required for manual mode
```

## API Endpoints

```
GET    /api/settings/tls            # TLS status (cert expiry, mode)
POST   /api/settings/tls/test       # test ACME provisioning
```

## Port Handling

- Default listen port: 443.
- If port 443 is occupied at startup, fall back to 8443.
- Configurable permanently via `listen` in `config.yaml`.
- `CAP_NET_BIND_SERVICE` capability allows binding to port 443 without root.
