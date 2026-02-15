# Updates & Versioning

> **Purpose**: Specifies the self-update mechanism, version detection from GitHub releases, binary replacement, semantic versioning policy, and the project roadmap.
>
> **Related docs**: [../features/install-script.md](../features/install-script.md), [../architecture/project-structure.md](../architecture/project-structure.md)
>
> **Implements**: `internal/updater/updater.go`, `cmd/wgpilot` (update subcommand)

---

## Self-Update Mechanism

The binary can self-update by fetching the latest release from GitHub:

```
wgpilot update                      # download latest, replace binary, restart service
    --check                       # just check for updates, don't install
    --version=STRING              # install specific version
```

### Update Flow

1. Query GitHub Releases API for latest version.
2. Compare against current version (embedded at build time via ldflags).
3. Download the binary for the current architecture.
4. Replace the running binary at `/usr/local/bin/wgpilot`.
5. Signal systemd to restart the service.

The same install script can also handle updates — re-running it downloads the latest binary and restarts the service.

## GoReleaser Configuration

```yaml
# .goreleaser.yaml
builds:
  - binary: wgpilot
    main: ./cmd/wgpilot
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64, arm]
    goarm: ['7']
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
```

## GitHub Actions — CI (on every PR)

```yaml
name: CI
on: [pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: cd frontend && npm ci && npm run lint && npm run build
      - run: go vet ./...
      - run: go test ./...
```

## GitHub Actions — Release (on tag push)

```yaml
name: Release
on:
  push:
    tags: ['v*']
jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: cd frontend && npm ci && npm run build
      - uses: goreleaser/goreleaser-action@v6
        with:
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Version Information

Embedded at build time via Go ldflags:

```go
var (
    version = "dev"
    commit  = "unknown"
    date    = "unknown"
)
```

Set by GoReleaser:

```yaml
ldflags:
  - -s -w
  - -X main.version={{.Version}}
  - -X main.commit={{.Commit}}
  - -X main.date={{.Date}}
```

Printed via:

```
wgpilot version
# wgpilot v0.3.0 (abc1234) built 2026-02-15
```

## Versioning Policy

Semantic versioning. Stay below v1.0.0 until API surface and data model are stable. Breaking changes are expected and fine in v0.x.

## Roadmap

| Version | Scope |
|---|---|
| v0.1.0 | Install script + Go backend + API + basic UI + VPN gateway mode |
| v0.2.0 | Site-to-site support |
| v0.3.0 | Hub with peer routing |
| v0.4.0 | Multi-network + network bridging |
| v0.5.0 | Import existing WireGuard configs |
| v0.6.0 | Dashboard with live stats + transfer charts |
| v0.7.0 | Alerts + audit log |
| v0.8.0 | Self-update mechanism |
| v0.9.0 | Prometheus metrics + health endpoint |
| v1.0.0 | Stable, production-ready |
