# Security Specification

> **Purpose**: Define all security controls, authentication flows, input validation rules, and hardening measures. wgpilot manages network infrastructure with elevated privileges — security is not optional.
>
> **Related docs**: [auth.md](../features/auth.md), [service.md](../operations/service.md), [tls.md](../operations/tls.md), [api-surface.md](../architecture/api-surface.md)
>
> **Implements**: `internal/auth/`, `internal/server/middleware/`, `internal/config/`

---

## Authentication

### JWT-Based Session Auth

- On successful login, the server issues a JWT stored in an `HttpOnly`, `Secure`, `SameSite=Strict` cookie. No localStorage tokens.
- JWT secret: 256-bit random value generated on first run, stored in SQLite `settings` table. Never written to config files or environment.
- JWT lifetime: configurable, default 24 hours. After expiry, user must re-login.
- JWT payload: `{ sub: user_id, username: string, role: "admin", iat: unix, exp: unix }`. Minimal claims — no sensitive data in the token.
- Refresh: no refresh tokens for v1.0. Session simply expires. This is a management tool with few users, not a consumer app.

### Login Flow

```
POST /api/auth/login
Body: { "username": "admin", "password": "..." }

→ 200 { "user": { "id": 1, "username": "admin" } }
  Set-Cookie: session=<jwt>; HttpOnly; Secure; SameSite=Strict; Path=/; Max-Age=86400

→ 401 { "error": "invalid credentials", "code": "INVALID_CREDENTIALS" }
```

### First-Run Bootstrap

The install script generates a one-time password (OTP). This OTP is bcrypt-hashed and stored in the `settings` table with key `setup_otp`. The setup wizard uses it to authenticate the first session:

```
POST /api/auth/setup
Body: { "otp": "<install password>", "username": "admin", "password": "<new password>" }

→ 201: admin account created, OTP deleted, JWT issued
→ 401: invalid OTP
→ 409: setup already completed
```

The OTP is single-use. After the admin account is created, the OTP row is deleted from the database.

### Password Requirements

- Minimum 10 characters
- No maximum length (bcrypt truncates at 72 bytes — warn the user if they exceed this)
- No character class requirements (length is more secure than complexity rules)
- Bcrypt cost factor: 12
- Passwords are NEVER logged, NEVER stored in plaintext, NEVER included in error messages

---

## Authorization

v1.0 has a single role: `admin`. All authenticated users are admins. Every API endpoint except the following requires a valid JWT:

**Public endpoints (no auth):**
- `GET /health` — health check
- `POST /api/auth/login` — login
- `POST /api/auth/setup` — first-run setup (disabled after completion)

**Admin-only endpoints (valid JWT required):**
- Everything else

Future versions may add roles (viewer, operator, admin) but v1.0 keeps it simple.

---

## Rate Limiting

### Login Endpoint

`POST /api/auth/login` is rate-limited per source IP:

- 5 attempts per minute per IP
- On limit exceeded: return `429 Too Many Requests` with `Retry-After` header
- Implementation: in-memory token bucket per IP, cleaned up every 10 minutes
- Failed attempts are logged at WARN with IP address

```go
type LoginRateLimiter struct {
    mu       sync.RWMutex
    attempts map[string][]time.Time // IP → timestamps
    limit    int                     // max attempts
    window   time.Duration           // time window
}
```

### API Endpoints

No rate limiting on authenticated API endpoints for v1.0. The admin is the only user — rate limiting would only hurt them. Revisit if multi-user support is added.

---

## Input Validation

Every API endpoint validates all input before processing. Validation errors return `400 Bad Request` with a `VALIDATION_ERROR` code and a `fields` array describing each violation.

```json
{
    "error": "validation failed",
    "code": "VALIDATION_ERROR",
    "request_id": "req_abc123",
    "fields": [
        { "field": "subnet", "message": "invalid CIDR notation" },
        { "field": "listen_port", "message": "must be between 1024 and 65535" }
    ]
}
```

### Validation Rules by Entity

**Network:**
- `name`: 1-64 chars, alphanumeric + spaces + hyphens + underscores
- `interface`: auto-generated (`wg0`, `wg1`, ...) — not user-settable
- `subnet`: valid IPv4 CIDR, private range (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16), prefix length /16 to /30
- `listen_port`: integer 1024-65535, not already in use
- `mode`: one of `gateway`, `site-to-site`, `hub-routed`
- `dns_servers`: array of valid IPv4/IPv6 addresses, max 3

**Peer:**
- `name`: 1-64 chars, alphanumeric + spaces + hyphens + underscores
- `role`: one of `client`, `site-gateway`
- `endpoint`: valid IP:port or hostname:port (optional for clients)
- `persistent_keepalive`: integer 0-65535 (0 means disabled)
- `site_networks`: array of valid CIDRs (only when role=site-gateway)
- `allowed_ips`: never user-provided directly — computed by the server based on mode and role
- `public_key`: valid WireGuard public key format (base64, 44 chars including `=` suffix) — only when importing, normally server-generated

**Settings:**
- `public_ip`: valid IPv4/IPv6 address
- `hostname`: valid FQDN or empty
- `dns_servers`: array of valid IPs

### Validation Implementation

Use a dedicated validation package with composable validators:

```go
func (s *Server) validateCreateNetwork(req CreateNetworkRequest) []FieldError {
    var errs []FieldError

    if !isValidName(req.Name) {
        errs = append(errs, FieldError{"name", "1-64 alphanumeric characters, spaces, hyphens, underscores"})
    }
    if !isValidPrivateCIDR(req.Subnet) {
        errs = append(errs, FieldError{"subnet", "must be a valid private IPv4 CIDR (/16 to /30)"})
    }
    if req.ListenPort < 1024 || req.ListenPort > 65535 {
        errs = append(errs, FieldError{"listen_port", "must be between 1024 and 65535"})
    }
    if !isValidMode(req.Mode) {
        errs = append(errs, FieldError{"mode", "must be gateway, site-to-site, or hub-routed"})
    }

    return errs
}
```

---

## Cryptographic Key Management

### WireGuard Keys

- **Server private keys**: generated via `wgtypes.GeneratePrivateKey()` (uses `crypto/rand`). Stored in SQLite encrypted with AES-256-GCM.
- **Peer private keys**: generated server-side, included in the downloadable `.conf` file or QR code, then stored encrypted in SQLite. The user is responsible for the `.conf` file after download.
- **Preshared keys**: optional, generated via `wgtypes.GenerateKey()` if enabled. Stored encrypted in SQLite.
- **Public keys**: derived from private keys. Stored in plaintext in SQLite (they're public).

### Encryption at Rest

Private keys and preshared keys stored in SQLite are encrypted with AES-256-GCM. The encryption key is derived from the JWT secret using HKDF-SHA256 with a fixed info string `"wgpilot-key-encryption"`.

```go
func deriveEncryptionKey(jwtSecret []byte) []byte {
    hkdf := hkdf.New(sha256.New, jwtSecret, nil, []byte("wgpilot-key-encryption"))
    key := make([]byte, 32)
    io.ReadFull(hkdf, key)
    return key
}
```

This means if the SQLite file is stolen without the running application, private keys are not readable.

### Key Rotation

Not supported in v1.0. The JWT secret (and by extension the key encryption key) is generated once and never rotated. Users who want rotation must back up, reinitialize, and restore. This is acceptable for v1.0 scope.

---

## HTTP Security Headers

Applied to every response via middleware:

```go
func securityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "0") // modern browsers don't need it, can cause issues
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

        // HSTS only when TLS is active
        if r.TLS != nil {
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }

        // CSP: only allow resources from same origin
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")

        next.ServeHTTP(w, r)
    })
}
```

---

## CORS

CORS is restricted to same-origin. The SPA is served from the same binary on the same origin, so no cross-origin requests are needed.

In dev mode (Vite proxy), the Go backend allows requests from `localhost:5173`:

```go
if devMode {
    w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
```

In production: no CORS headers at all (same-origin doesn't need them).

---

## Request Size Limits

- Maximum request body: 1MB (covers any reasonable API payload)
- Maximum URL length: 8KB
- Maximum header size: 32KB (Go's default)

Enforced at the HTTP server level:

```go
srv := &http.Server{
    MaxHeaderBytes: 32 << 10,
    ReadTimeout:    10 * time.Second,
    WriteTimeout:   30 * time.Second,
    IdleTimeout:    120 * time.Second,
}

// Body size limit middleware
func maxBodySize(limit int64) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            r.Body = http.MaxBytesReader(w, r.Body, limit)
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## SQLite Security

- Database file permissions: `0640` (owner read/write, group read, no world access)
- WAL mode enabled (concurrent reads, single writer — no corruption on crash)
- `PRAGMA journal_mode=WAL`
- `PRAGMA foreign_keys=ON` (enforce referential integrity)
- `PRAGMA busy_timeout=5000` (wait up to 5s for locks instead of failing immediately)

---

## Audit Logging

Every state-changing action is logged at INFO level with the authenticated user:

```go
logger.Info("peer_created",
    "user", currentUser.Username,
    "peer_id", peer.ID,
    "peer_name", peer.Name,
    "network_id", network.ID,
    "remote_addr", remoteAddr,
    "component", "audit",
)
```

Audit events: login, logout, failed_login, network_created, network_updated, network_deleted, peer_created, peer_updated, peer_deleted, peer_enabled, peer_disabled, settings_updated, setup_completed, backup_created, backup_restored.

---

## Filesystem Security

The systemd unit enforces filesystem restrictions:

- `ProtectHome=true` — cannot access /home
- `ProtectSystem=strict` — filesystem is read-only except for explicitly allowed paths
- `ReadWritePaths=/var/lib/wgpilot` — only the data directory is writable
- `ReadOnlyPaths=/etc/wgpilot` — config is readable but not writable at runtime
- `PrivateTmp=true` — isolated /tmp namespace

---

## Threat Model Summary

| Threat | Mitigation |
|---|---|
| Stolen SQLite file | Private keys encrypted with AES-256-GCM |
| Brute-force login | Rate limiting (5/min/IP), bcrypt cost 12 |
| Session hijacking | HttpOnly + Secure + SameSite=Strict cookies |
| XSS | CSP headers, no inline scripts, React escapes by default |
| Clickjacking | X-Frame-Options: DENY |
| CSRF | SameSite=Strict cookies, no CORS in production |
| Privilege escalation | Single admin role, capabilities instead of root |
| Log injection | Structured logging (slog), no user input in log format strings |
| SQL injection | Parameterized queries only, no string concatenation |
| Path traversal | No user-controlled file paths in any endpoint |
| Unauthorized API access | JWT required on all endpoints except /health and /login |
