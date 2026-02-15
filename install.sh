#!/usr/bin/env bash
#
# wgpilot install script
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/itsChris/wgpilot/master/install.sh | sudo bash
#   sudo bash install.sh --uninstall
#
set -euo pipefail

REPO="itsChris/wgpilot"
BINARY_NAME="wgpilot"
INSTALL_DIR="/usr/local/bin"
SERVICE_USER="wgpilot"
SERVICE_GROUP="wgpilot"
DATA_DIR="/var/lib/wgpilot"
CONFIG_DIR="/etc/wgpilot"
SERVICE_NAME="wgpilot"
SYSTEMD_UNIT="/etc/systemd/system/${SERVICE_NAME}.service"

# ── Colors ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
fatal() { err "$@"; exit 1; }

# ── Uninstall ─────────────────────────────────────────────────────────
do_uninstall() {
    info "Uninstalling wgpilot..."

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        info "Stopping ${SERVICE_NAME} service..."
        systemctl stop "${SERVICE_NAME}"
    fi

    if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
        info "Disabling ${SERVICE_NAME} service..."
        systemctl disable "${SERVICE_NAME}"
    fi

    if [ -f "${SYSTEMD_UNIT}" ]; then
        info "Removing systemd unit file..."
        rm -f "${SYSTEMD_UNIT}"
        systemctl daemon-reload
    fi

    if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        info "Removing binary..."
        rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    if [ -d "${CONFIG_DIR}" ]; then
        info "Removing config directory ${CONFIG_DIR}..."
        rm -rf "${CONFIG_DIR}"
    fi

    if [ -d "${DATA_DIR}" ]; then
        warn "Data directory ${DATA_DIR} preserved (contains database)."
        warn "Remove manually if no longer needed: rm -rf ${DATA_DIR}"
    fi

    if id "${SERVICE_USER}" &>/dev/null; then
        info "Removing system user ${SERVICE_USER}..."
        userdel "${SERVICE_USER}" 2>/dev/null || true
    fi

    if getent group "${SERVICE_GROUP}" &>/dev/null; then
        info "Removing system group ${SERVICE_GROUP}..."
        groupdel "${SERVICE_GROUP}" 2>/dev/null || true
    fi

    # Remove sysctl config
    if [ -f /etc/sysctl.d/99-wgpilot.conf ]; then
        info "Removing sysctl configuration..."
        rm -f /etc/sysctl.d/99-wgpilot.conf
    fi

    ok "wgpilot uninstalled successfully."
    info "Note: WireGuard packages were not removed."
    exit 0
}

# ── Parse arguments ───────────────────────────────────────────────────
for arg in "$@"; do
    case "${arg}" in
        --uninstall)
            # Root check before uninstall too
            if [ "$(id -u)" -ne 0 ]; then
                fatal "This script must be run as root. Try: sudo bash $0 --uninstall"
            fi
            do_uninstall
            ;;
        *)
            fatal "Unknown argument: ${arg}. Usage: install.sh [--uninstall]"
            ;;
    esac
done

# ── 1. Root check ────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    fatal "This script must be run as root. Try: curl -fsSL https://... | sudo bash"
fi

info "Starting wgpilot installation..."

# ── 2. Detect OS ──────────────────────────────────────────────────────
detect_os() {
    if [ ! -f /etc/os-release ]; then
        fatal "Cannot detect OS: /etc/os-release not found. Supported: Ubuntu, Debian, Fedora, CentOS, Rocky, AlmaLinux."
    fi

    # shellcheck source=/dev/null
    . /etc/os-release

    OS_ID="${ID}"
    OS_VERSION="${VERSION_ID:-unknown}"

    case "${OS_ID}" in
        ubuntu|debian)
            PKG_MANAGER="apt"
            ;;
        fedora)
            PKG_MANAGER="dnf"
            ;;
        centos|rocky|almalinux|rhel)
            PKG_MANAGER="dnf"
            if ! command -v dnf &>/dev/null; then
                PKG_MANAGER="yum"
            fi
            ;;
        *)
            fatal "Unsupported OS: ${OS_ID}. Supported: Ubuntu, Debian, Fedora, CentOS, Rocky, AlmaLinux."
            ;;
    esac

    ok "Detected OS: ${OS_ID} ${OS_VERSION} (package manager: ${PKG_MANAGER})"
}

# ── 3. Detect architecture ───────────────────────────────────────────
detect_arch() {
    MACHINE="$(uname -m)"
    case "${MACHINE}" in
        x86_64)
            ARCH="linux_amd64"
            ;;
        aarch64)
            ARCH="linux_arm64"
            ;;
        armv7l)
            ARCH="linux_arm7"
            ;;
        *)
            fatal "Unsupported architecture: ${MACHINE}. Supported: x86_64, aarch64, armv7l."
            ;;
    esac
    ok "Detected architecture: ${MACHINE} (binary suffix: ${ARCH})"
}

# ── 4. Check/install WireGuard ────────────────────────────────────────
ensure_wireguard() {
    # Check if WireGuard kernel module is available
    if modinfo wireguard &>/dev/null || [ -d /sys/module/wireguard ]; then
        ok "WireGuard kernel module available"
    else
        # Kernel 5.6+ has WireGuard built-in — check kernel version
        KERNEL_MAJOR=$(uname -r | cut -d. -f1)
        KERNEL_MINOR=$(uname -r | cut -d. -f2)
        if [ "${KERNEL_MAJOR}" -gt 5 ] || { [ "${KERNEL_MAJOR}" -eq 5 ] && [ "${KERNEL_MINOR}" -ge 6 ]; }; then
            ok "Kernel $(uname -r) has built-in WireGuard support"
        else
            warn "WireGuard kernel module not found. Attempting to load..."
            modprobe wireguard 2>/dev/null || true
            if ! modinfo wireguard &>/dev/null; then
                warn "WireGuard module not available. It will be installed with wireguard-tools."
            fi
        fi
    fi

    # Install wireguard-tools if wg command is not available
    if command -v wg &>/dev/null; then
        ok "wireguard-tools already installed"
        return
    fi

    info "Installing wireguard-tools..."
    case "${PKG_MANAGER}" in
        apt)
            apt-get update -qq
            apt-get install -y -qq wireguard-tools >/dev/null
            ;;
        dnf)
            dnf install -y -q wireguard-tools >/dev/null
            ;;
        yum)
            yum install -y -q wireguard-tools >/dev/null
            ;;
    esac

    if command -v wg &>/dev/null; then
        ok "wireguard-tools installed"
    else
        fatal "Failed to install wireguard-tools"
    fi
}

# ── 5. Enable IP forwarding ──────────────────────────────────────────
enable_ip_forwarding() {
    local changed=false

    if [ "$(cat /proc/sys/net/ipv4/ip_forward)" -ne 1 ]; then
        sysctl -w net.ipv4.ip_forward=1 >/dev/null
        changed=true
    fi

    if [ -f /proc/sys/net/ipv6/conf/all/forwarding ] && \
       [ "$(cat /proc/sys/net/ipv6/conf/all/forwarding)" -ne 1 ]; then
        sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null
        changed=true
    fi

    # Make persistent
    cat > /etc/sysctl.d/99-wgpilot.conf <<'SYSCTL'
# wgpilot: enable IP forwarding for WireGuard
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
SYSCTL

    if [ "${changed}" = true ]; then
        ok "IP forwarding enabled (persistent via /etc/sysctl.d/99-wgpilot.conf)"
    else
        ok "IP forwarding already enabled"
    fi
}

# ── 6. Download binary ───────────────────────────────────────────────
download_binary() {
    info "Fetching latest release from GitHub..."

    if ! command -v curl &>/dev/null; then
        fatal "curl is required but not installed"
    fi

    # Get the latest release tag
    LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
    RELEASE_JSON=$(curl -fsSL "${LATEST_URL}") || fatal "Failed to fetch latest release info from GitHub"
    TAG=$(echo "${RELEASE_JSON}" | grep -o '"tag_name":\s*"[^"]*"' | head -1 | cut -d'"' -f4)

    if [ -z "${TAG}" ]; then
        fatal "Could not determine latest release version"
    fi

    info "Latest version: ${TAG}"

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY_NAME}_${ARCH}"
    info "Downloading ${DOWNLOAD_URL}..."

    TMP_BIN=$(mktemp)
    trap 'rm -f "${TMP_BIN}"' EXIT

    curl -fsSL -o "${TMP_BIN}" "${DOWNLOAD_URL}" || fatal "Failed to download binary"

    chmod +x "${TMP_BIN}"

    # Verify the binary runs
    if ! "${TMP_BIN}" version &>/dev/null; then
        fatal "Downloaded binary failed verification (could not run 'version' command)"
    fi

    mv "${TMP_BIN}" "${INSTALL_DIR}/${BINARY_NAME}"
    trap - EXIT
    ok "Binary installed to ${INSTALL_DIR}/${BINARY_NAME}"
}

# ── 7. Create system user ────────────────────────────────────────────
create_user() {
    if id "${SERVICE_USER}" &>/dev/null; then
        ok "System user ${SERVICE_USER} already exists"
        return
    fi

    useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
    ok "Created system user: ${SERVICE_USER}"
}

# ── 8. Create directories ────────────────────────────────────────────
create_directories() {
    mkdir -p "${DATA_DIR}"
    chown "${SERVICE_USER}:${SERVICE_GROUP}" "${DATA_DIR}"
    chmod 750 "${DATA_DIR}"

    mkdir -p "${CONFIG_DIR}"
    chown root:"${SERVICE_GROUP}" "${CONFIG_DIR}"
    chmod 750 "${CONFIG_DIR}"

    ok "Directories created: ${DATA_DIR}, ${CONFIG_DIR}"
}

# ── 9. Generate default config ───────────────────────────────────────
generate_config() {
    local config_file="${CONFIG_DIR}/config.yaml"
    if [ -f "${config_file}" ]; then
        ok "Config file already exists: ${config_file}"
        return
    fi

    cat > "${config_file}" <<'CONFIG'
# wgpilot configuration
# See documentation for all options.

server:
  listen: 0.0.0.0:443

database:
  path: /var/lib/wgpilot/wgpilot.db

tls:
  mode: self-signed              # self-signed | acme | manual
  domain: ""                     # required for acme mode
  cert_file: ""                  # required for manual mode
  key_file: ""                   # required for manual mode

auth:
  session_ttl: 24h
  bcrypt_cost: 12

logging:
  level: info                    # debug | info | warn | error
  format: json                   # json | text
CONFIG

    chown root:"${SERVICE_GROUP}" "${config_file}"
    chmod 640 "${config_file}"
    ok "Default config generated: ${config_file}"
}

# ── 10. Install systemd unit ─────────────────────────────────────────
install_systemd_unit() {
    cat > "${SYSTEMD_UNIT}" <<UNIT
[Unit]
Description=WireGuard Web UI
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=${INSTALL_DIR}/${BINARY_NAME} serve --config=${CONFIG_DIR}/config.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=5
WatchdogSec=30

User=${SERVICE_USER}
Group=${SERVICE_GROUP}

AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=true

ReadWritePaths=${DATA_DIR}
ReadOnlyPaths=${CONFIG_DIR}
ProtectHome=true
ProtectSystem=strict
PrivateTmp=true

RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_UNIX
ProtectKernelTunables=false
ProtectKernelModules=true
ProtectKernelLogs=true
LockPersonality=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native

Environment=WGPILOT_DATA_DIR=${DATA_DIR}
Environment=WGPILOT_CONFIG=${CONFIG_DIR}/config.yaml
Environment=WGPILOT_LOG_LEVEL=info

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    ok "Systemd unit installed: ${SYSTEMD_UNIT}"
}

# ── 11. Initialize wgpilot ───────────────────────────────────────────
initialize_wgpilot() {
    info "Initializing wgpilot (generating OTP and JWT secret)..."

    # Run init as the service user so file ownership is correct
    INIT_OUTPUT=$(su -s /bin/sh "${SERVICE_USER}" -c \
        "${INSTALL_DIR}/${BINARY_NAME} init --data-dir=${DATA_DIR}" 2>&1) \
        || fatal "wgpilot init failed: ${INIT_OUTPUT}"

    # Extract OTP from output
    OTP=$(echo "${INIT_OUTPUT}" | grep -oP '(?<=One-time setup password: ).*' || true)
    if [ -z "${OTP}" ]; then
        warn "Could not extract OTP from init output"
    fi

    ok "wgpilot initialized"
}

# ── 12. Configure firewall ────────────────────────────────────────────
configure_firewall() {
    echo ""
    echo -e "${YELLOW}wgpilot listens on port 443 (HTTPS).${NC}"
    echo -n "Open port 443 in the firewall for public access? [y/N] "
    read -r REPLY < /dev/tty
    echo ""

    if [[ ! "${REPLY}" =~ ^[Yy]$ ]]; then
        info "Skipping firewall configuration. You may need to open port 443 manually."
        return
    fi

    # Detect firewall and open port 443/tcp
    if command -v ufw &>/dev/null && ufw status | grep -q "active"; then
        ufw allow 443/tcp >/dev/null
        ok "Firewall: opened port 443/tcp (ufw)"
    elif command -v firewall-cmd &>/dev/null && systemctl is-active --quiet firewalld; then
        firewall-cmd --permanent --add-port=443/tcp >/dev/null
        firewall-cmd --reload >/dev/null
        ok "Firewall: opened port 443/tcp (firewalld)"
    elif command -v iptables &>/dev/null; then
        iptables -C INPUT -p tcp --dport 443 -j ACCEPT 2>/dev/null || \
            iptables -I INPUT -p tcp --dport 443 -j ACCEPT
        # Persist if iptables-save is available
        if command -v iptables-save &>/dev/null; then
            iptables-save > /etc/iptables/rules.v4 2>/dev/null || true
        fi
        ok "Firewall: opened port 443/tcp (iptables)"
    else
        warn "No supported firewall detected (ufw, firewalld, iptables). Open port 443 manually if needed."
    fi
}

# ── 13. Enable and start service ─────────────────────────────────────
start_service() {
    systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1
    systemctl start "${SERVICE_NAME}"
    ok "Service ${SERVICE_NAME} enabled and started"
}

# ── 14. Detect public IP and print info ──────────────────────────────
print_info() {
    local public_ip
    public_ip=$(curl -fsSL -4 https://ifconfig.me 2>/dev/null || \
                curl -fsSL -4 https://api.ipify.org 2>/dev/null || \
                curl -fsSL -4 https://icanhazip.com 2>/dev/null || \
                echo "unknown")

    echo ""
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}  wgpilot installed successfully!${NC}"
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "  Web UI:   ${BLUE}https://${public_ip}${NC}"
    echo ""
    if [ -n "${OTP:-}" ]; then
        echo -e "  One-time setup password: ${YELLOW}${OTP}${NC}"
    fi
    echo ""
    echo -e "  ${RED}⚠  Change this password immediately after first login.${NC}"
    echo ""
    echo -e "  Service:  systemctl status ${SERVICE_NAME}"
    echo -e "  Logs:     journalctl -u ${SERVICE_NAME} -f"
    echo ""
}

# ── Run installation steps ────────────────────────────────────────────
detect_os
detect_arch
ensure_wireguard
enable_ip_forwarding
download_binary
create_user
create_directories
generate_config
install_systemd_unit
initialize_wgpilot
configure_firewall
start_service
print_info
