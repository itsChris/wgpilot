package testutil

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MockNFTManager implements nft.NFTableManager for testing.
// It tracks applied rules in memory and records all method calls.
type MockNFTManager struct {
	mu    sync.Mutex
	Calls []MockCall

	// Internal rule tracking.
	NATRules     map[string]string // iface -> subnet
	ForwardRules map[string]bool   // iface -> enabled
	BridgeRules  map[string]string // "ifaceA:ifaceB" (sorted) -> direction

	// Override functions for custom behavior.
	AddNATMasqueradeFn           func(iface, subnet string) error
	RemoveNATMasqueradeFn        func(iface string) error
	EnableInterPeerForwardingFn  func(iface string) error
	DisableInterPeerForwardingFn func(iface string) error
	AddNetworkBridgeFn           func(ifaceA, ifaceB, direction string) error
	RemoveNetworkBridgeFn        func(ifaceA, ifaceB string) error
	DumpRulesFn                  func() (string, error)
}

// NewMockNFTManager creates a MockNFTManager with initialized maps.
func NewMockNFTManager() *MockNFTManager {
	return &MockNFTManager{
		NATRules:     make(map[string]string),
		ForwardRules: make(map[string]bool),
		BridgeRules:  make(map[string]string),
	}
}

func (m *MockNFTManager) AddNATMasquerade(iface, subnet string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "AddNATMasquerade", Args: []any{iface, subnet}})
	m.NATRules[iface] = subnet
	m.mu.Unlock()
	if m.AddNATMasqueradeFn != nil {
		return m.AddNATMasqueradeFn(iface, subnet)
	}
	return nil
}

func (m *MockNFTManager) RemoveNATMasquerade(iface string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "RemoveNATMasquerade", Args: []any{iface}})
	delete(m.NATRules, iface)
	m.mu.Unlock()
	if m.RemoveNATMasqueradeFn != nil {
		return m.RemoveNATMasqueradeFn(iface)
	}
	return nil
}

func (m *MockNFTManager) EnableInterPeerForwarding(iface string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "EnableInterPeerForwarding", Args: []any{iface}})
	m.ForwardRules[iface] = true
	m.mu.Unlock()
	if m.EnableInterPeerForwardingFn != nil {
		return m.EnableInterPeerForwardingFn(iface)
	}
	return nil
}

func (m *MockNFTManager) DisableInterPeerForwarding(iface string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "DisableInterPeerForwarding", Args: []any{iface}})
	delete(m.ForwardRules, iface)
	m.mu.Unlock()
	if m.DisableInterPeerForwardingFn != nil {
		return m.DisableInterPeerForwardingFn(iface)
	}
	return nil
}

func (m *MockNFTManager) AddNetworkBridge(ifaceA, ifaceB, direction string) error {
	m.mu.Lock()
	key := sortedBridgeKey(ifaceA, ifaceB)
	m.Calls = append(m.Calls, MockCall{Method: "AddNetworkBridge", Args: []any{ifaceA, ifaceB, direction}})
	m.BridgeRules[key] = direction
	m.mu.Unlock()
	if m.AddNetworkBridgeFn != nil {
		return m.AddNetworkBridgeFn(ifaceA, ifaceB, direction)
	}
	return nil
}

func (m *MockNFTManager) RemoveNetworkBridge(ifaceA, ifaceB string) error {
	m.mu.Lock()
	key := sortedBridgeKey(ifaceA, ifaceB)
	m.Calls = append(m.Calls, MockCall{Method: "RemoveNetworkBridge", Args: []any{ifaceA, ifaceB}})
	delete(m.BridgeRules, key)
	m.mu.Unlock()
	if m.RemoveNetworkBridgeFn != nil {
		return m.RemoveNetworkBridgeFn(ifaceA, ifaceB)
	}
	return nil
}

func (m *MockNFTManager) DumpRules() (string, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "DumpRules"})
	if m.DumpRulesFn != nil {
		m.mu.Unlock()
		return m.DumpRulesFn()
	}
	result := m.formatRulesLocked()
	m.mu.Unlock()
	return result, nil
}

// CallMethods returns the method names of all recorded calls.
func (m *MockNFTManager) CallMethods() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	methods := make([]string, len(m.Calls))
	for i, c := range m.Calls {
		methods[i] = c.Method
	}
	return methods
}

// formatRulesLocked returns a simple text summary of tracked rules.
// Must be called with m.mu held.
func (m *MockNFTManager) formatRulesLocked() string {
	var lines []string
	for iface, subnet := range m.NATRules {
		lines = append(lines, fmt.Sprintf("NAT %s %s", iface, subnet))
	}
	for iface := range m.ForwardRules {
		lines = append(lines, fmt.Sprintf("FORWARD %s", iface))
	}
	for key, dir := range m.BridgeRules {
		lines = append(lines, fmt.Sprintf("BRIDGE %s %s", key, dir))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func sortedBridgeKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + ":" + b
}
