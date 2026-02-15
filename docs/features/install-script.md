# Install Script

> **Purpose**: Specifies the one-liner install script behavior: OS detection, prerequisites, binary download, service setup, and initial configuration.
>
> **Related docs**: [../operations/service.md](../operations/service.md), [../operations/updates.md](../operations/updates.md), [first-run.md](first-run.md)
>
> **Implements**: `install.sh`

---

## One-Liner

```bash
curl -fsSL https://raw.githubusercontent.com/itsChris/wgpilot/main/install.sh | sudo bash
```

Or with a custom domain:

```bash
curl -fsSL https://get.wgpilot.dev | sudo bash
```

## What the Script Does

```
1.  Check root
2.  Detect architecture (amd64, arm64, arm7)
3.  Detect OS (Ubuntu, Debian, Fedora, CentOS, Rocky, Alma)
4.  Check kernel version for WireGuard support
5.  Install wireguard-tools if needed (package manager)
6.  Enable IP forwarding (sysctl)
7.  Download latest binary from GitHub Releases
8.  Install to /usr/local/bin/wgpilot
9.  Create system user (wg-webui)
10. Create directories (/var/lib/wg-webui, /etc/wg-webui)
11. Generate default config.yaml
12. Generate one-time admin password
13. Initialize database (wgpilot init)
14. Install systemd unit file
15. Enable and start service
16. Print URL + credentials
```

## Supported Platforms

| OS | Versions |
|---|---|
| Ubuntu | 20.04, 22.04, 24.04 |
| Debian | 11, 12 |
| Fedora | 39, 40, 41 |
| Rocky Linux | 8, 9 |
| AlmaLinux | 8, 9 |
| CentOS Stream | 8, 9 |

## Architectures

| Arch | `uname -m` | Binary suffix |
|---|---|---|
| x86_64 | `x86_64` | `linux_amd64` |
| ARM64 | `aarch64` | `linux_arm64` |
| ARMv7 | `armv7l` | `linux_arm7` |
