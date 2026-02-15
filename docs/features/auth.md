# Authentication & Authorization

> **Purpose**: Specifies the login flow, JWT session management, bcrypt password storage, first-run token authentication, rate limiting, and security hardening measures.
>
> **Related docs**: [../architecture/api-surface.md](../architecture/api-surface.md), [first-run.md](first-run.md)
>
> **Implements**: `internal/auth/`, `internal/server/middleware.go`, `internal/server/handlers/auth.go`

---

## Login Flow

1. User submits username + password to `POST /api/auth/login`.
2. Server verifies bcrypt hash.
3. Server issues JWT (HS256, signed with a random secret generated on first run and stored in settings).
4. JWT stored in `httpOnly`, `secure`, `sameSite=strict` cookie.
5. JWT expiry: 24 hours (configurable via `auth.session_ttl`).
6. On each API request, middleware validates JWT from cookie.
7. If JWT is expired, return 401. Frontend redirects to login.

## JWT Payload

```json
{
    "sub": 1,
    "username": "admin",
    "role": "admin",
    "iat": 1739000000,
    "exp": 1739086400
}
```

## First-Run Authentication

During setup (before `setup_complete=true`), the install script generates a one-time password. This authenticates the first session via `POST /api/setup/admin` which accepts:

```json
{
    "install_token": "Kx9mP2vQ7nB4wR1j",
    "username": "admin",
    "password": "user-chosen-password"
}
```

The install token is invalidated after the admin account is created.

## Password Storage

- bcrypt with cost factor 12.
- Minimum password length: 10 characters (enforced in UI and API).

## Security Hardening

### Network

- HTTPS only (self-signed, ACME, or manual cert). No HTTP listener.
- HTTP → HTTPS redirect on port 80 (optional, for ACME challenges).
- `Strict-Transport-Security` header.
- CORS disabled (SPA is same-origin).

### Authentication

- bcrypt (cost 12) for password storage.
- JWT in httpOnly, secure, sameSite=strict cookie.
- One-time install token for first-run auth.
- Rate limiting on login endpoint (5 attempts per minute per IP).

### API

- All endpoints require authentication (except `/health`, `/metrics`, `/api/auth/login`, `/api/setup/*`).
- CSRF protection via `SameSite=strict` cookie + `Origin` header check.
- Input validation on all endpoints (reject unknown fields, enforce types/ranges).
- SQL injection prevention via parameterized queries.
- XSS prevention via React's default escaping + `Content-Security-Policy` header.

### System

- Dedicated system user (`wg-webui`), no shell, no home directory.
- Minimal Linux capabilities (`CAP_NET_ADMIN`, `CAP_NET_BIND_SERVICE`).
- Filesystem restrictions via systemd (`ProtectSystem=strict`, `ProtectHome=true`).
- Private keys encrypted at rest in SQLite (AES-256-GCM, key derived from a master secret in config).
- Audit log for all administrative actions.

### Firewall

- All nftables rules in a dedicated `wgpilot` table — no modification of existing rules.
- Rules are scoped to WireGuard interfaces only.
