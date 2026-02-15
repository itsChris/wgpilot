# Testing Strategy

> **Purpose**: Define what gets tested, how, at what level, and to what coverage standard. Ensures Claude Code writes the right tests — not too many, not too few, and in the right places.
>
> **Related docs**: [tech-stack.md](../architecture/tech-stack.md), [data-model.md](../architecture/data-model.md), [api-surface.md](../architecture/api-surface.md)
>
> **Implements**: `*_test.go` files across all `internal/` packages, `internal/testutil/`

---

## Testing Pyramid

```
         ╱  E2E  ╲           ← manual / future: full install script on VM
        ╱──────────╲
       ╱ Integration╲        ← API handlers + real SQLite + mocked kernel
      ╱──────────────╲
     ╱   Unit Tests    ╲     ← pure logic, no I/O, fast
    ╱────────────────────╲
```

Focus for v1.0: unit tests and integration tests. No E2E automation yet.

---

## Unit Tests

### What to Unit Test

Every function that contains logic (branching, transformation, validation, computation). Specifically:

- **IP address allocation** — given a subnet and existing allocations, next IP is correct, exhaustion is detected
- **AllowedIPs calculation** — given a topology mode and peer role, correct AllowedIPs are generated
- **Config file generation** — given a network + peer, correct `.conf` file is templated
- **QR code generation** — given a config string, valid QR image is produced
- **Input validation** — subnet format, port ranges, key format, endpoint format, name constraints
- **Error classification** — `classifyNetlinkError` returns correct hints
- **Auth logic** — JWT generation, validation, expiry, bcrypt hashing/verification
- **Config loading** — YAML parsing, env var override, CLI flag override, priority chain
- **Ring buffer** — write, read, wraparound, concurrency safety
- **Migration ordering** — migrations are applied in sequence, idempotent re-application

### What NOT to Unit Test

- Simple struct constructors with no logic
- Direct `wgctrl` or `netlink` calls (these are integration-tested with mocks)
- HTTP routing (tested at integration level)
- Frontend components (deferred post-v1.0)

### Unit Test Conventions

```go
// File: internal/wg/ip_alloc_test.go
package wg

import "testing"

func TestAllocateNextIP_EmptySubnet(t *testing.T) {
    pool := NewIPPool("10.0.0.0/24")
    ip, err := pool.Allocate()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // .1 is reserved for the server
    if ip.String() != "10.0.0.2" {
        t.Errorf("got %s, want 10.0.0.2", ip)
    }
}

func TestAllocateNextIP_Exhausted(t *testing.T) {
    pool := NewIPPool("10.0.0.0/30") // only 2 usable IPs
    pool.Reserve("10.0.0.1")          // server
    pool.Reserve("10.0.0.2")          // peer 1
    _, err := pool.Allocate()
    if err == nil {
        t.Fatal("expected IP exhaustion error")
    }
}
```

**Table-driven tests** for functions with multiple input/output variations:

```go
func TestClassifyNetlinkError(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        wantHint string
    }{
        {
            name:     "permission denied",
            err:      errors.New("operation not permitted"),
            wantHint: "missing CAP_NET_ADMIN",
        },
        {
            name:     "interface exists",
            err:      errors.New("file exists"),
            wantHint: "interface already exists",
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            hint := classifyNetlinkError(tt.err)
            if !strings.Contains(hint, tt.wantHint) {
                t.Errorf("got %q, want substring %q", hint, tt.wantHint)
            }
        })
    }
}
```

---

## Integration Tests

### What to Integration Test

API handlers with a real SQLite database (in-memory) and mocked kernel interfaces. These tests verify the full request → validate → business logic → persist → respond cycle.

### Mocking Strategy

Define interfaces for kernel-level operations. Production uses real implementations. Tests use mocks.

```go
// internal/wg/interfaces.go
type WireGuardController interface {
    Devices() ([]*wgtypes.Device, error)
    Device(name string) (*wgtypes.Device, error)
    ConfigureDevice(name string, cfg wgtypes.Config) error
    Close() error
}

type LinkManager interface {
    LinkAdd(link netlink.Link) error
    LinkDel(link netlink.Link) error
    LinkSetUp(link netlink.Link) error
    LinkSetDown(link netlink.Link) error
    AddrAdd(link netlink.Link, addr *netlink.Addr) error
    RouteAdd(route *netlink.Route) error
}

type NFTableManager interface {
    AddNATRule(iface string, subnet string) error
    RemoveNATRule(iface string) error
    AddForwardRule(srcIface, dstIface string) error
    RemoveForwardRule(srcIface, dstIface string) error
}
```

```go
// internal/testutil/mocks.go
type MockWGController struct {
    devices map[string]*wgtypes.Device
    mu      sync.RWMutex
}

func (m *MockWGController) Device(name string) (*wgtypes.Device, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    dev, ok := m.devices[name]
    if !ok {
        return nil, fmt.Errorf("no such device: %s", name)
    }
    return dev, nil
}

// ... implement all interface methods with in-memory state
```

### Integration Test Pattern

```go
// internal/server/peers_test.go
package server_test

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestCreatePeer_Success(t *testing.T) {
    // Setup: in-memory DB, mock WG controller, test server
    db := testutil.NewTestDB(t)
    wg := testutil.NewMockWGController()
    srv := server.New(db, wg, testLogger(t))

    // Seed: create a network first
    testutil.SeedNetwork(t, db, testutil.DefaultNetwork())

    // Act
    body := `{"name":"My Phone","role":"client","full_tunnel":true}`
    req := httptest.NewRequest("POST", "/api/networks/1/peers", strings.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+testutil.AdminToken())
    rec := httptest.NewRecorder()
    srv.ServeHTTP(rec, req)

    // Assert HTTP response
    if rec.Code != http.StatusCreated {
        t.Fatalf("status %d, want 201. body: %s", rec.Code, rec.Body.String())
    }

    // Assert DB state
    peers, _ := db.ListPeersByNetworkID(1)
    if len(peers) != 1 {
        t.Fatalf("got %d peers, want 1", len(peers))
    }
    if peers[0].Name != "My Phone" {
        t.Errorf("peer name %q, want %q", peers[0].Name, "My Phone")
    }

    // Assert WG state (mock was called correctly)
    dev, _ := wg.Device("wg0")
    if len(dev.Peers) != 1 {
        t.Fatalf("got %d WG peers, want 1", len(dev.Peers))
    }
}

func TestCreatePeer_DuplicatePublicKey(t *testing.T) {
    // ... test that duplicate key returns 409 Conflict
}

func TestCreatePeer_IPExhausted(t *testing.T) {
    // ... test that full subnet returns 507/422 with IP_POOL_EXHAUSTED code
}

func TestCreatePeer_Unauthorized(t *testing.T) {
    // ... test that missing/invalid JWT returns 401
}
```

### Test Helpers (`internal/testutil/`)

```go
// testutil/db.go
func NewTestDB(t *testing.T) *db.DB {
    t.Helper()
    d, err := db.New(":memory:", testLogger(t), false)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { d.Close() })
    return d
}

// testutil/seed.go
func DefaultNetwork() *model.Network { ... }
func DefaultPeer() *model.Peer { ... }
func SeedNetwork(t *testing.T, db *db.DB, n *model.Network) { ... }
func SeedPeer(t *testing.T, db *db.DB, p *model.Peer) { ... }

// testutil/auth.go
func AdminToken() string { ... }        // valid JWT for tests
func ExpiredToken() string { ... }      // expired JWT for tests
func InvalidToken() string { ... }      // malformed JWT for tests

// testutil/logger.go
func TestLogger(t *testing.T) *slog.Logger { ... } // logs to t.Log
```

---

## Critical Test Scenarios

These specific scenarios MUST have tests because they represent the highest-risk failure modes:

### Network Management
- Create network → interface exists in mock, DB has record, nftables rules applied
- Delete network → interface removed, peers removed, rules cleaned up
- Create network with port already in use → error, no partial state
- Create two networks with overlapping subnets → error
- Startup reconciliation: DB has network, kernel doesn't → interface recreated
- Startup reconciliation: kernel has interface, DB doesn't → logged as orphaned, not deleted

### Peer Management
- Create peer → WG peer added, IP allocated, config downloadable
- Delete peer → WG peer removed, IP returned to pool
- IP allocation is sequential and skips server IP (.1)
- IP allocation detects exhaustion correctly
- Peer config generation produces valid WireGuard config syntax
- AllowedIPs differ correctly by topology mode (gateway vs site-to-site vs hub-routed)

### Auth
- Valid login → JWT issued, session created
- Invalid password → 401, failed attempt logged, no JWT
- Expired JWT → 401 on protected endpoint
- Rate limiting → 429 after 5 failed attempts in 1 minute
- First-run one-time password → works once, disabled after admin account created

### Data Integrity
- Concurrent peer creation on same network → no duplicate IPs
- Delete network with active peers → cascading cleanup, no orphans
- Database migration from version N to N+1 → data preserved

---

## Test Execution

```bash
# All tests
go test ./...

# Specific package
go test ./internal/wg/...

# With verbose output
go test -v ./internal/server/...

# With race detector (run in CI)
go test -race ./...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

CI runs `go test -race ./...` on every PR. Coverage is reported but not gated — quality over metrics.

---

## Test Data

- Use deterministic test data (fixed keys, known subnets) so test failures are reproducible
- Never rely on external services or network access in tests
- Use `t.TempDir()` for any file-based tests (auto-cleaned)
- Use `t.Parallel()` where tests are independent (most unit tests)
