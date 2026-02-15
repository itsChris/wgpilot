package nft

import (
	"fmt"
	"log/slog"
	"sync"
)

// Manager implements NFTableManager by tracking rules in memory and
// syncing them to the kernel via an Applier.
type Manager struct {
	applier Applier
	logger  *slog.Logger
	devMode bool

	mu    sync.Mutex
	rules map[string]Rule
}

// NewManager creates an NFTManager with the given dependencies.
func NewManager(applier Applier, logger *slog.Logger, devMode bool) (*Manager, error) {
	if applier == nil {
		return nil, fmt.Errorf("new nft manager: applier is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("new nft manager: logger is required")
	}
	return &Manager{
		applier: applier,
		logger:  logger.With("component", "nft"),
		devMode: devMode,
		rules:   make(map[string]Rule),
	}, nil
}

// NewTestManager creates a Manager with a no-op applier for testing.
func NewTestManager(logger *slog.Logger, devMode bool) (*Manager, error) {
	return NewManager(noopApplier{}, logger, devMode)
}

// AddNATMasquerade adds a masquerade rule for the given interface and subnet.
func (m *Manager) AddNATMasquerade(iface, subnet string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("nft_add_nat_start",
		"interface", iface,
		"subnet", subnet,
		"operation", "add_nat_masquerade",
	)

	rule := Rule{
		Kind:   RuleNATMasquerade,
		Iface:  iface,
		Subnet: subnet,
	}
	key := ruleKey(rule)

	if _, exists := m.rules[key]; exists {
		m.logger.Debug("nft_add_nat_idempotent",
			"interface", iface,
			"subnet", subnet,
			"operation", "add_nat_masquerade",
		)
		return nil
	}

	m.rules[key] = rule

	if err := m.apply(); err != nil {
		delete(m.rules, key)
		m.logger.Error("nft_apply_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "add_nat_masquerade",
			"interface", iface,
			"subnet", subnet,
		)
		return fmt.Errorf("add NAT masquerade for %s: %w", iface, err)
	}

	m.logDevDump("add_nat_masquerade", iface)
	m.logger.Info("nft_nat_added",
		"interface", iface,
		"subnet", subnet,
		"operation", "add_nat_masquerade",
	)
	return nil
}

// RemoveNATMasquerade removes the masquerade rule for the given interface.
func (m *Manager) RemoveNATMasquerade(iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("nft_remove_nat_start",
		"interface", iface,
		"operation", "remove_nat_masquerade",
	)

	key := "nat:" + iface
	old, exists := m.rules[key]
	if !exists {
		m.logger.Debug("nft_remove_nat_not_found",
			"interface", iface,
			"operation", "remove_nat_masquerade",
		)
		return nil
	}

	delete(m.rules, key)

	if err := m.apply(); err != nil {
		m.rules[key] = old
		m.logger.Error("nft_apply_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "remove_nat_masquerade",
			"interface", iface,
		)
		return fmt.Errorf("remove NAT masquerade for %s: %w", iface, err)
	}

	m.logDevDump("remove_nat_masquerade", iface)
	m.logger.Info("nft_nat_removed",
		"interface", iface,
		"operation", "remove_nat_masquerade",
	)
	return nil
}

// EnableInterPeerForwarding allows peers on the same interface to reach each other.
func (m *Manager) EnableInterPeerForwarding(iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("nft_enable_forward_start",
		"interface", iface,
		"operation", "enable_inter_peer_forwarding",
	)

	rule := Rule{
		Kind:  RuleInterPeerForward,
		Iface: iface,
	}
	key := ruleKey(rule)

	if _, exists := m.rules[key]; exists {
		m.logger.Debug("nft_enable_forward_idempotent",
			"interface", iface,
			"operation", "enable_inter_peer_forwarding",
		)
		return nil
	}

	m.rules[key] = rule

	if err := m.apply(); err != nil {
		delete(m.rules, key)
		m.logger.Error("nft_apply_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "enable_inter_peer_forwarding",
			"interface", iface,
		)
		return fmt.Errorf("enable inter-peer forwarding for %s: %w", iface, err)
	}

	m.logDevDump("enable_inter_peer_forwarding", iface)
	m.logger.Info("nft_forward_enabled",
		"interface", iface,
		"operation", "enable_inter_peer_forwarding",
	)
	return nil
}

// DisableInterPeerForwarding removes the inter-peer forwarding rule.
func (m *Manager) DisableInterPeerForwarding(iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("nft_disable_forward_start",
		"interface", iface,
		"operation", "disable_inter_peer_forwarding",
	)

	key := "forward:" + iface
	old, exists := m.rules[key]
	if !exists {
		m.logger.Debug("nft_disable_forward_not_found",
			"interface", iface,
			"operation", "disable_inter_peer_forwarding",
		)
		return nil
	}

	delete(m.rules, key)

	if err := m.apply(); err != nil {
		m.rules[key] = old
		m.logger.Error("nft_apply_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "disable_inter_peer_forwarding",
			"interface", iface,
		)
		return fmt.Errorf("disable inter-peer forwarding for %s: %w", iface, err)
	}

	m.logDevDump("disable_inter_peer_forwarding", iface)
	m.logger.Info("nft_forward_disabled",
		"interface", iface,
		"operation", "disable_inter_peer_forwarding",
	)
	return nil
}

// AddNetworkBridge adds forwarding rules between two WireGuard interfaces.
func (m *Manager) AddNetworkBridge(ifaceA, ifaceB, direction string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("nft_add_bridge_start",
		"interface_a", ifaceA,
		"interface_b", ifaceB,
		"direction", direction,
		"operation", "add_network_bridge",
	)

	if !validDirection(direction) {
		return fmt.Errorf("add network bridge %s <-> %s: invalid direction %q", ifaceA, ifaceB, direction)
	}

	rule := Rule{
		Kind:      RuleBridgeForward,
		Iface:     ifaceA,
		IfaceB:    ifaceB,
		Direction: direction,
	}
	key := ruleKey(rule)
	old, hadOld := m.rules[key]
	m.rules[key] = rule

	if err := m.apply(); err != nil {
		if hadOld {
			m.rules[key] = old
		} else {
			delete(m.rules, key)
		}
		m.logger.Error("nft_apply_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "add_network_bridge",
			"interface_a", ifaceA,
			"interface_b", ifaceB,
			"direction", direction,
		)
		return fmt.Errorf("add network bridge %s <-> %s: %w", ifaceA, ifaceB, err)
	}

	m.logDevDump("add_network_bridge", ifaceA)
	m.logger.Info("nft_bridge_added",
		"interface_a", ifaceA,
		"interface_b", ifaceB,
		"direction", direction,
		"operation", "add_network_bridge",
	)
	return nil
}

// RemoveNetworkBridge removes forwarding rules between two interfaces.
func (m *Manager) RemoveNetworkBridge(ifaceA, ifaceB string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("nft_remove_bridge_start",
		"interface_a", ifaceA,
		"interface_b", ifaceB,
		"operation", "remove_network_bridge",
	)

	key := bridgeKey(ifaceA, ifaceB)
	old, exists := m.rules[key]
	if !exists {
		m.logger.Debug("nft_remove_bridge_not_found",
			"interface_a", ifaceA,
			"interface_b", ifaceB,
			"operation", "remove_network_bridge",
		)
		return nil
	}

	delete(m.rules, key)

	if err := m.apply(); err != nil {
		m.rules[key] = old
		m.logger.Error("nft_apply_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "remove_network_bridge",
			"interface_a", ifaceA,
			"interface_b", ifaceB,
		)
		return fmt.Errorf("remove network bridge %s <-> %s: %w", ifaceA, ifaceB, err)
	}

	m.logDevDump("remove_network_bridge", ifaceA)
	m.logger.Info("nft_bridge_removed",
		"interface_a", ifaceA,
		"interface_b", ifaceB,
		"operation", "remove_network_bridge",
	)
	return nil
}

// DumpRules returns a human-readable nftables-style representation of all active rules.
func (m *Manager) DumpRules() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dumpRules(), nil
}

// apply sends the current rule set to the kernel via the Applier.
// Must be called with m.mu held.
func (m *Manager) apply() error {
	rules := make([]Rule, 0, len(m.rules))
	for _, r := range m.rules {
		rules = append(rules, r)
	}
	return m.applier.Apply(rules)
}

// dumpRules formats the current rule set. Must be called with m.mu held.
func (m *Manager) dumpRules() string {
	return formatRuleset(m.rules)
}

// logDevDump logs the full ruleset after a change in dev mode.
// Must be called with m.mu held.
func (m *Manager) logDevDump(action, iface string) {
	if !m.devMode {
		return
	}
	m.logger.Debug("nft_ruleset_after_change",
		"interface", iface,
		"action", action,
		"ruleset", m.dumpRules(),
	)
}
