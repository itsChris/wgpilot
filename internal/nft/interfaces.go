package nft

// NFTableManager manages nftables firewall rules for WireGuard interfaces.
// All rules are managed in a dedicated "wgpilot" nftables table to avoid
// conflicts with existing firewall rules.
type NFTableManager interface {
	// AddNATMasquerade adds a masquerade rule for the given interface and subnet.
	// Packets entering via iface and leaving via any other interface are masqueraded.
	// Calling this with an already-configured interface is idempotent.
	AddNATMasquerade(iface, subnet string) error

	// RemoveNATMasquerade removes the masquerade rule for the given interface.
	// Returns nil if no rule exists for the interface.
	RemoveNATMasquerade(iface string) error

	// EnableInterPeerForwarding allows peers on the same interface to route
	// traffic through the server to each other (hub-routed mode).
	// Calling this with an already-configured interface is idempotent.
	EnableInterPeerForwarding(iface string) error

	// DisableInterPeerForwarding removes the inter-peer forwarding rule.
	// Returns nil if no rule exists for the interface.
	DisableInterPeerForwarding(iface string) error

	// AddNetworkBridge adds forwarding rules between two WireGuard interfaces.
	// Direction must be "a_to_b", "b_to_a", or "bidirectional".
	// If a bridge already exists for the same interface pair, it is updated.
	AddNetworkBridge(ifaceA, ifaceB, direction string) error

	// RemoveNetworkBridge removes forwarding rules between two interfaces.
	// The order of interfaces does not matter. Returns nil if no bridge exists.
	RemoveNetworkBridge(ifaceA, ifaceB string) error

	// DumpRules returns a human-readable nftables-style representation
	// of all active rules in the wgpilot table.
	DumpRules() (string, error)
}

// Applier abstracts the kernel nftables operations for testability.
// The real implementation translates rules into google/nftables API calls.
type Applier interface {
	// Apply replaces all rules in the wgpilot nftables table with the given set.
	// An empty slice removes all rules (deletes the table).
	Apply(rules []Rule) error
}
