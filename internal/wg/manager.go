package wg

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/itsChris/wgpilot/internal/logging"
)

// Manager coordinates WireGuard interface and peer lifecycle operations.
type Manager struct {
	wg     WireGuardController
	link   LinkManager
	logger *slog.Logger

	mu sync.Mutex
}

// NewManager creates a Manager with the given dependencies.
func NewManager(wg WireGuardController, link LinkManager, logger *slog.Logger) (*Manager, error) {
	if wg == nil {
		return nil, fmt.Errorf("new manager: wireguard controller is required")
	}
	if link == nil {
		return nil, fmt.Errorf("new manager: link manager is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("new manager: logger is required")
	}
	return &Manager{
		wg:     wg,
		link:   link,
		logger: logger.With("component", "wg"),
	}, nil
}

// CreateInterface creates a WireGuard network interface, assigns an address,
// configures the device, and brings it up.
func (m *Manager) CreateInterface(ctx context.Context, network NetworkConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createInterface(ctx, network)
}

func (m *Manager) createInterface(ctx context.Context, network NetworkConfig) error {
	l := m.ctxLogger(ctx)
	l.Debug("create_interface_start",
		"interface", network.Interface,
		"subnet", network.Subnet,
		"listen_port", network.ListenPort,
		"operation", "create_interface",
	)

	// Step 1: Create WireGuard link
	if err := m.link.CreateWireGuardLink(network.Interface); err != nil {
		l.Error("link_add_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_interface",
			"interface", network.Interface,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("create interface %s: link add: %w", network.Interface, err)
	}
	l.Debug("link_added", "interface", network.Interface, "operation", "create_interface")

	// Step 2: Assign address (server IP from subnet)
	_, subnet, err := net.ParseCIDR(network.Subnet)
	if err != nil {
		_ = m.link.DeleteLink(network.Interface)
		l.Error("parse_subnet_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_interface",
			"subnet", network.Subnet,
		)
		return fmt.Errorf("create interface %s: parse subnet %s: %w", network.Interface, network.Subnet, err)
	}
	serverIP := firstUsableIP(subnet)
	ones, _ := subnet.Mask.Size()
	addr := fmt.Sprintf("%s/%d", serverIP.String(), ones)

	if err := m.link.AddAddress(network.Interface, addr); err != nil {
		_ = m.link.DeleteLink(network.Interface)
		l.Error("addr_add_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_interface",
			"interface", network.Interface,
			"address", addr,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("create interface %s: assign address %s: %w", network.Interface, addr, err)
	}
	l.Debug("addr_assigned", "interface", network.Interface, "address", addr, "operation", "create_interface")

	// Step 3: Configure WireGuard device (private key, listen port)
	cfg := DeviceConfig{
		PrivateKey: network.PrivateKey,
		ListenPort: network.ListenPort,
	}
	if err := m.wg.ConfigureDevice(network.Interface, cfg); err != nil {
		_ = m.link.DeleteLink(network.Interface)
		l.Error("configure_device_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_interface",
			"interface", network.Interface,
			"listen_port", network.ListenPort,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("create interface %s: configure device: %w", network.Interface, err)
	}
	l.Debug("device_configured", "interface", network.Interface, "listen_port", network.ListenPort, "operation", "create_interface")

	// Step 4: Bring interface up
	if err := m.link.SetLinkUp(network.Interface); err != nil {
		_ = m.link.DeleteLink(network.Interface)
		l.Error("link_set_up_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_interface",
			"interface", network.Interface,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("create interface %s: bring up: %w", network.Interface, err)
	}
	l.Debug("link_up", "interface", network.Interface, "operation", "create_interface")

	l.Info("interface_created",
		"interface", network.Interface,
		"address", addr,
		"listen_port", network.ListenPort,
		"operation", "create_interface",
	)
	return nil
}

// DeleteInterface tears down a WireGuard interface: removes all peers,
// brings the interface down, and deletes it.
func (m *Manager) DeleteInterface(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteInterface(ctx, name)
}

func (m *Manager) deleteInterface(ctx context.Context, name string) error {
	l := m.ctxLogger(ctx)
	l.Debug("delete_interface_start", "interface", name, "operation", "delete_interface")

	// Step 1: Get current device to find peers
	dev, err := m.wg.Device(name)
	if err != nil {
		l.Error("get_device_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "delete_interface",
			"interface", name,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("delete interface %s: get device: %w", name, err)
	}

	// Step 2: Remove all peers
	if len(dev.Peers) > 0 {
		var removePeers []WGPeerConfig
		for _, p := range dev.Peers {
			removePeers = append(removePeers, WGPeerConfig{
				PublicKey: p.PublicKey,
				Remove:    true,
			})
		}
		if err := m.wg.ConfigureDevice(name, DeviceConfig{Peers: removePeers}); err != nil {
			l.Error("remove_peers_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "delete_interface",
				"interface", name,
				"peer_count", len(dev.Peers),
				"hint", ClassifyNetlinkError(err),
			)
			return fmt.Errorf("delete interface %s: remove peers: %w", name, err)
		}
		l.Debug("peers_removed", "interface", name, "count", len(dev.Peers), "operation", "delete_interface")
	}

	// Step 3: Bring interface down
	if err := m.link.SetLinkDown(name); err != nil {
		l.Error("link_set_down_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "delete_interface",
			"interface", name,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("delete interface %s: bring down: %w", name, err)
	}
	l.Debug("link_down", "interface", name, "operation", "delete_interface")

	// Step 4: Delete link
	if err := m.link.DeleteLink(name); err != nil {
		l.Error("link_del_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "delete_interface",
			"interface", name,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("delete interface %s: delete link: %w", name, err)
	}
	l.Debug("link_deleted", "interface", name, "operation", "delete_interface")

	l.Info("interface_deleted", "interface", name, "operation", "delete_interface")
	return nil
}

// AddPeer adds a peer to a WireGuard interface.
func (m *Manager) AddPeer(ctx context.Context, iface string, peer PeerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addPeer(ctx, iface, peer)
}

func (m *Manager) addPeer(ctx context.Context, iface string, peer PeerConfig) error {
	l := m.ctxLogger(ctx)
	l.Debug("add_peer_start",
		"interface", iface,
		"peer_name", peer.Name,
		"public_key", peer.PublicKey,
		"allowed_ips", peer.AllowedIPs,
		"operation", "add_peer",
	)

	peerCfg, err := m.buildPeerConfig(peer, false)
	if err != nil {
		l.Error("build_peer_config_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "add_peer",
			"interface", iface,
			"peer_name", peer.Name,
		)
		return fmt.Errorf("add peer %s to %s: %w", peer.Name, iface, err)
	}

	if err := m.wg.ConfigureDevice(iface, DeviceConfig{Peers: []WGPeerConfig{peerCfg}}); err != nil {
		l.Error("configure_device_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "add_peer",
			"interface", iface,
			"peer_name", peer.Name,
			"public_key", peer.PublicKey,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("add peer %s to %s: %w", peer.Name, iface, err)
	}

	l.Info("peer_added",
		"interface", iface,
		"peer_name", peer.Name,
		"public_key", peer.PublicKey,
		"allowed_ips", peer.AllowedIPs,
		"operation", "add_peer",
	)
	return nil
}

// RemovePeer removes a peer from a WireGuard interface by its public key.
func (m *Manager) RemovePeer(ctx context.Context, iface string, publicKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.ctxLogger(ctx)
	l.Debug("remove_peer_start",
		"interface", iface,
		"public_key", publicKey,
		"operation", "remove_peer",
	)

	peerCfg := WGPeerConfig{
		PublicKey: publicKey,
		Remove:    true,
	}

	if err := m.wg.ConfigureDevice(iface, DeviceConfig{Peers: []WGPeerConfig{peerCfg}}); err != nil {
		l.Error("configure_device_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "remove_peer",
			"interface", iface,
			"public_key", publicKey,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("remove peer %s from %s: %w", publicKey, iface, err)
	}

	l.Info("peer_removed",
		"interface", iface,
		"public_key", publicKey,
		"operation", "remove_peer",
	)
	return nil
}

// UpdatePeer updates an existing peer's configuration on a WireGuard interface.
func (m *Manager) UpdatePeer(ctx context.Context, iface string, peer PeerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.ctxLogger(ctx)
	l.Debug("update_peer_start",
		"interface", iface,
		"peer_name", peer.Name,
		"public_key", peer.PublicKey,
		"allowed_ips", peer.AllowedIPs,
		"operation", "update_peer",
	)

	peerCfg, err := m.buildPeerConfig(peer, true)
	if err != nil {
		l.Error("build_peer_config_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "update_peer",
			"interface", iface,
			"peer_name", peer.Name,
		)
		return fmt.Errorf("update peer %s on %s: %w", peer.Name, iface, err)
	}

	if err := m.wg.ConfigureDevice(iface, DeviceConfig{Peers: []WGPeerConfig{peerCfg}}); err != nil {
		l.Error("configure_device_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "update_peer",
			"interface", iface,
			"peer_name", peer.Name,
			"public_key", peer.PublicKey,
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("update peer %s on %s: %w", peer.Name, iface, err)
	}

	l.Info("peer_updated",
		"interface", iface,
		"peer_name", peer.Name,
		"public_key", peer.PublicKey,
		"allowed_ips", peer.AllowedIPs,
		"operation", "update_peer",
	)
	return nil
}

// PeerStatus returns the runtime status of all peers on a WireGuard interface.
func (m *Manager) PeerStatus(iface string) ([]PeerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("peer_status_start", "interface", iface, "operation", "peer_status")

	dev, err := m.wg.Device(iface)
	if err != nil {
		m.logger.Error("get_device_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "peer_status",
			"interface", iface,
			"hint", ClassifyNetlinkError(err),
		)
		return nil, fmt.Errorf("peer status for %s: %w", iface, err)
	}

	statuses := make([]PeerStatus, 0, len(dev.Peers))
	for _, p := range dev.Peers {
		var allowedIPs []string
		for _, aip := range p.AllowedIPs {
			allowedIPs = append(allowedIPs, aip.String())
		}

		online := !p.LastHandshake.IsZero() && time.Since(p.LastHandshake) < 3*time.Minute

		statuses = append(statuses, PeerStatus{
			PublicKey:     p.PublicKey,
			Endpoint:      p.Endpoint,
			LastHandshake: p.LastHandshake,
			TransferRx:    p.ReceiveBytes,
			TransferTx:    p.TransmitBytes,
			AllowedIPs:    allowedIPs,
			Online:        online,
		})
	}

	m.logger.Debug("peer_status_complete",
		"interface", iface,
		"peer_count", len(statuses),
		"operation", "peer_status",
	)
	return statuses, nil
}

// DetectInterfaces returns the names of all existing WireGuard interfaces.
func (m *Manager) DetectInterfaces() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	devs, err := m.wg.Devices()
	if err != nil {
		return nil, fmt.Errorf("detect interfaces: %w", err)
	}

	var names []string
	for _, d := range devs {
		names = append(names, d.Name)
	}
	return names, nil
}

// buildPeerConfig constructs a WGPeerConfig from a PeerConfig.
func (m *Manager) buildPeerConfig(peer PeerConfig, updateOnly bool) (WGPeerConfig, error) {
	allowedIPs, err := parseAllowedIPs(peer.AllowedIPs)
	if err != nil {
		return WGPeerConfig{}, fmt.Errorf("parse allowed IPs %q: %w", peer.AllowedIPs, err)
	}

	peerCfg := WGPeerConfig{
		PublicKey:         peer.PublicKey,
		UpdateOnly:        updateOnly,
		ReplaceAllowedIPs: true,
		AllowedIPs:        allowedIPs,
	}

	if peer.PresharedKey != "" {
		peerCfg.PresharedKey = peer.PresharedKey
	}

	if peer.Endpoint != "" {
		peerCfg.Endpoint = peer.Endpoint
	}

	if peer.PersistentKeepalive > 0 {
		peerCfg.PersistentKeepaliveInterval = time.Duration(peer.PersistentKeepalive) * time.Second
	}

	return peerCfg, nil
}

// ctxLogger returns a logger enriched with context attributes (request_id, task_id).
func (m *Manager) ctxLogger(ctx context.Context) *slog.Logger {
	attrs := logging.LogAttrsFromContext(ctx)
	if len(attrs) == 0 {
		return m.logger
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return m.logger.With(args...)
}

// parseAllowedIPs parses a comma-separated list of CIDRs into net.IPNet slices.
func parseAllowedIPs(s string) ([]net.IPNet, error) {
	if s == "" {
		return nil, nil
	}

	parts := splitAndTrim(s)
	var result []net.IPNet
	for _, p := range parts {
		_, ipNet, err := net.ParseCIDR(p)
		if err != nil {
			return nil, fmt.Errorf("parse CIDR %q: %w", p, err)
		}
		result = append(result, *ipNet)
	}
	return result, nil
}

// splitAndTrim splits a string by comma and trims whitespace from each part.
func splitAndTrim(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			// Trim whitespace
			j := 0
			for j < len(part) && (part[j] == ' ' || part[j] == '\t') {
				j++
			}
			k := len(part)
			for k > j && (part[k-1] == ' ' || part[k-1] == '\t') {
				k--
			}
			if j < k {
				result = append(result, part[j:k])
			}
			start = i + 1
		}
	}
	return result
}
