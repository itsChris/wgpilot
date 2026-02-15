package wg

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/itsChris/wgpilot/internal/logging"
)

// Reconcile compares database state against kernel state and corrects any mismatches.
// The database is always the source of truth.
func (m *Manager) Reconcile(ctx context.Context, store NetworkStore) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := m.ctxLogger(ctx)
	l.Info("reconciliation_start", "operation", "reconcile")

	// Step 1: Get all networks from DB
	networks, err := store.ListNetworks(ctx)
	if err != nil {
		l.Error("list_networks_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "reconcile",
		)
		return fmt.Errorf("reconcile: list networks: %w", err)
	}

	// Step 2: Get all existing WG devices from kernel
	devices, err := m.wg.Devices()
	if err != nil {
		l.Error("list_devices_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "reconcile",
			"hint", ClassifyNetlinkError(err),
		)
		return fmt.Errorf("reconcile: list devices: %w", err)
	}

	deviceMap := make(map[string]*DeviceInfo, len(devices))
	for _, d := range devices {
		deviceMap[d.Name] = d
	}

	dbIfaceNames := make(map[string]bool, len(networks))

	// Step 3: For each DB network, ensure kernel matches
	for _, network := range networks {
		dbIfaceNames[network.Interface] = true
		dev := deviceMap[network.Interface]

		if !network.Enabled {
			if dev != nil {
				l.Warn("reconcile_disabled_network_has_interface",
					"network_id", network.ID,
					"interface", network.Interface,
					"action", "tearing_down",
					"operation", "reconcile",
				)
				if err := m.deleteInterface(ctx, network.Interface); err != nil {
					l.Error("reconcile_teardown_failed",
						"error", err,
						"error_type", fmt.Sprintf("%T", err),
						"network_id", network.ID,
						"interface", network.Interface,
						"operation", "reconcile",
					)
				}
			}
			continue
		}

		if dev == nil {
			// Interface doesn't exist in kernel — create it
			l.Warn("reconcile_missing_interface",
				"network_id", network.ID,
				"interface", network.Interface,
				"action", "recreating",
				"operation", "reconcile",
			)
			if err := m.createInterface(ctx, network); err != nil {
				l.Error("reconcile_create_interface_failed",
					"error", err,
					"error_type", fmt.Sprintf("%T", err),
					"network_id", network.ID,
					"interface", network.Interface,
					"operation", "reconcile",
				)
				continue
			}
		}

		// Sync peers
		dbPeers, err := store.ListPeersByNetworkID(ctx, network.ID)
		if err != nil {
			l.Error("reconcile_list_peers_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"network_id", network.ID,
				"operation", "reconcile",
			)
			continue
		}

		m.syncPeers(ctx, l, network.Interface, network.ID, dev, dbPeers)
	}

	// Step 4: Check for orphaned interfaces (in kernel but not in DB)
	for _, d := range devices {
		if !dbIfaceNames[d.Name] {
			l.Warn("reconcile_orphaned_interface",
				"interface", d.Name,
				"peer_count", len(d.Peers),
				"action", "ignored_not_managed",
				"operation", "reconcile",
			)
		}
	}

	l.Info("reconciliation_complete", "operation", "reconcile")
	return nil
}

// syncPeers compares DB peers against kernel peers for a single interface
// and corrects mismatches.
func (m *Manager) syncPeers(ctx context.Context, l *slog.Logger, iface string, networkID int64, dev *DeviceInfo, dbPeers []PeerConfig) {
	// Build kernel peer map by public key
	kernelPeers := make(map[string]WGPeerInfo)
	if dev != nil {
		for _, p := range dev.Peers {
			kernelPeers[p.PublicKey] = p
		}
	}

	// Log peer count mismatch
	if dev != nil && len(dbPeers) != len(dev.Peers) {
		l.Warn("reconcile_peer_count_mismatch",
			"network_id", networkID,
			"interface", iface,
			"db_peers", len(dbPeers),
			"kernel_peers", len(dev.Peers),
			"action", "syncing",
			"operation", "reconcile",
		)
	}

	// Check each DB peer
	dbPeerKeys := make(map[string]bool, len(dbPeers))
	for _, dbPeer := range dbPeers {
		dbPeerKeys[dbPeer.PublicKey] = true

		if !dbPeer.Enabled {
			// Disabled peer — remove from kernel if present
			if _, found := kernelPeers[dbPeer.PublicKey]; found {
				l.Warn("reconcile_disabled_peer_in_kernel",
					"peer_id", dbPeer.ID,
					"peer_name", dbPeer.Name,
					"network_id", networkID,
					"action", "removing_from_kernel",
					"operation", "reconcile",
				)
				if err := m.removePeerLocked(iface, dbPeer.PublicKey); err != nil {
					l.Error("reconcile_remove_disabled_peer_failed",
						"error", err,
						"peer_id", dbPeer.ID,
						"operation", "reconcile",
					)
				}
			}
			continue
		}

		kernelPeer, found := kernelPeers[dbPeer.PublicKey]
		if !found {
			// Peer missing from kernel — add it
			l.Warn("reconcile_missing_peer",
				"peer_id", dbPeer.ID,
				"peer_name", dbPeer.Name,
				"network_id", networkID,
				"action", "adding_to_kernel",
				"operation", "reconcile",
			)
			if err := m.addPeer(ctx, iface, dbPeer); err != nil {
				l.Error("reconcile_add_peer_failed",
					"error", err,
					"error_type", fmt.Sprintf("%T", err),
					"peer_id", dbPeer.ID,
					"peer_name", dbPeer.Name,
					"operation", "reconcile",
				)
			}
			continue
		}

		// Peer exists — check AllowedIPs match
		if !allowedIPsMatch(dbPeer.AllowedIPs, kernelPeer.AllowedIPs) {
			l.Warn("reconcile_allowed_ips_mismatch",
				"peer_id", dbPeer.ID,
				"peer_name", dbPeer.Name,
				"db_allowed_ips", dbPeer.AllowedIPs,
				"kernel_allowed_ips", formatIPNets(kernelPeer.AllowedIPs),
				"action", "updating_kernel",
				"operation", "reconcile",
			)
			if err := m.updatePeerLocked(iface, dbPeer); err != nil {
				l.Error("reconcile_update_peer_failed",
					"error", err,
					"peer_id", dbPeer.ID,
					"operation", "reconcile",
				)
			}
		}

		// Log endpoint mismatch at debug (endpoints are dynamic, not corrected)
		if dbPeer.Endpoint != "" && dbPeer.Endpoint != kernelPeer.Endpoint {
			l.Debug("reconcile_endpoint_mismatch",
				"peer_id", dbPeer.ID,
				"db_endpoint", dbPeer.Endpoint,
				"kernel_endpoint", kernelPeer.Endpoint,
				"action", "no_action_dynamic",
				"operation", "reconcile",
			)
		}
	}

	// Check for kernel peers not in DB (orphaned peers on managed interface)
	if dev != nil {
		for _, kp := range dev.Peers {
			if !dbPeerKeys[kp.PublicKey] {
				l.Warn("reconcile_orphaned_peer",
					"interface", iface,
					"public_key", kp.PublicKey,
					"network_id", networkID,
					"action", "removing_from_kernel",
					"operation", "reconcile",
				)
				if err := m.removePeerLocked(iface, kp.PublicKey); err != nil {
					l.Error("reconcile_remove_orphan_failed",
						"error", err,
						"public_key", kp.PublicKey,
						"operation", "reconcile",
					)
				}
			}
		}
	}
}

// removePeerLocked removes a peer without acquiring the mutex (caller must hold it).
func (m *Manager) removePeerLocked(iface string, publicKey string) error {
	peerCfg := WGPeerConfig{
		PublicKey: publicKey,
		Remove:    true,
	}
	if err := m.wg.ConfigureDevice(iface, DeviceConfig{Peers: []WGPeerConfig{peerCfg}}); err != nil {
		return fmt.Errorf("remove peer %s from %s: %w", publicKey, iface, err)
	}
	return nil
}

// updatePeerLocked updates a peer without acquiring the mutex (caller must hold it).
func (m *Manager) updatePeerLocked(iface string, peer PeerConfig) error {
	allowedIPs, err := parseAllowedIPs(peer.AllowedIPs)
	if err != nil {
		return fmt.Errorf("update peer %s: parse allowed IPs: %w", peer.Name, err)
	}
	peerCfg := WGPeerConfig{
		PublicKey:         peer.PublicKey,
		UpdateOnly:        true,
		ReplaceAllowedIPs: true,
		AllowedIPs:        allowedIPs,
	}
	if err := m.wg.ConfigureDevice(iface, DeviceConfig{Peers: []WGPeerConfig{peerCfg}}); err != nil {
		return fmt.Errorf("update peer %s on %s: %w", peer.Name, iface, err)
	}
	return nil
}

// allowedIPsMatch compares DB allowed IPs string against kernel AllowedIPs.
func allowedIPsMatch(dbAllowedIPs string, kernelAllowedIPs []net.IPNet) bool {
	dbNets, err := parseAllowedIPs(dbAllowedIPs)
	if err != nil {
		return false
	}

	if len(dbNets) != len(kernelAllowedIPs) {
		return false
	}

	// Build a set of kernel AllowedIPs for comparison
	kernelSet := make(map[string]bool, len(kernelAllowedIPs))
	for _, k := range kernelAllowedIPs {
		kernelSet[k.String()] = true
	}

	for _, d := range dbNets {
		if !kernelSet[d.String()] {
			return false
		}
	}

	return true
}

// formatIPNets formats a slice of net.IPNet as a comma-separated string.
func formatIPNets(nets []net.IPNet) string {
	if len(nets) == 0 {
		return ""
	}
	s := nets[0].String()
	for _, n := range nets[1:] {
		s += ", " + n.String()
	}
	return s
}

// ReconcileBridges reconciles bridge nftables rules against the database.
// It uses the NFTableManager interface to add bridge forwarding rules for
// all enabled bridges. This is separate from WG device reconciliation because
// bridge rules span nft (not wg) and the two packages are independent.
func ReconcileBridges(ctx context.Context, store NetworkStore, nftMgr interface {
	AddNetworkBridge(ifaceA, ifaceB, direction string) error
}, logger *slog.Logger) error {
	l := logger.With("component", "reconcile_bridges")

	bridges, err := store.ListBridges(ctx)
	if err != nil {
		l.Error("list_bridges_failed",
			"error", err,
			"operation", "reconcile_bridges",
		)
		return fmt.Errorf("reconcile bridges: list bridges: %w", err)
	}

	if len(bridges) == 0 {
		l.Debug("no_bridges_to_reconcile", "operation", "reconcile_bridges")
		return nil
	}

	for _, bridge := range bridges {
		if !bridge.Enabled {
			l.Debug("reconcile_bridge_disabled",
				"bridge_id", bridge.ID,
				"interface_a", bridge.InterfaceA,
				"interface_b", bridge.InterfaceB,
				"operation", "reconcile_bridges",
			)
			continue
		}

		l.Info("reconcile_bridge_apply",
			"bridge_id", bridge.ID,
			"interface_a", bridge.InterfaceA,
			"interface_b", bridge.InterfaceB,
			"direction", bridge.Direction,
			"operation", "reconcile_bridges",
		)

		if err := nftMgr.AddNetworkBridge(bridge.InterfaceA, bridge.InterfaceB, bridge.Direction); err != nil {
			l.Error("reconcile_bridge_add_failed",
				"error", err,
				"bridge_id", bridge.ID,
				"interface_a", bridge.InterfaceA,
				"interface_b", bridge.InterfaceB,
				"operation", "reconcile_bridges",
			)
		}
	}

	l.Info("reconcile_bridges_complete",
		"bridge_count", len(bridges),
		"operation", "reconcile_bridges",
	)
	return nil
}

// contextForReconcile creates a context with a reconcile task ID.
func ContextForReconcile(ctx context.Context) context.Context {
	return logging.WithTaskID(ctx, logging.GenerateTaskID("reconcile"))
}
