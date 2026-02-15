package nft

import (
	"fmt"
	"sort"
	"strings"
)

// RuleKind identifies the type of nftables rule.
type RuleKind string

const (
	RuleNATMasquerade    RuleKind = "nat_masquerade"
	RuleInterPeerForward RuleKind = "inter_peer_forward"
	RuleBridgeForward    RuleKind = "bridge_forward"
)

// Rule represents a managed nftables rule in the wgpilot table.
type Rule struct {
	Kind      RuleKind
	Iface     string
	IfaceB    string // second interface, for bridge rules
	Subnet    string // subnet CIDR, for NAT rules
	Direction string // "a_to_b", "b_to_a", "bidirectional" for bridge rules
}

// ruleKey returns a unique identifier for the rule, used for deduplication.
func ruleKey(r Rule) string {
	switch r.Kind {
	case RuleNATMasquerade:
		return "nat:" + r.Iface
	case RuleInterPeerForward:
		return "forward:" + r.Iface
	case RuleBridgeForward:
		return "bridge:" + sortedPair(r.Iface, r.IfaceB)
	default:
		return fmt.Sprintf("unknown:%s:%s", r.Kind, r.Iface)
	}
}

// bridgeKey returns the canonical key for a bridge between two interfaces.
func bridgeKey(ifaceA, ifaceB string) string {
	return "bridge:" + sortedPair(ifaceA, ifaceB)
}

// sortedPair returns "a:b" where a <= b lexicographically.
func sortedPair(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + ":" + b
}

// validDirection reports whether d is a valid bridge direction.
func validDirection(d string) bool {
	return d == "a_to_b" || d == "b_to_a" || d == "bidirectional"
}

// forwardEntry represents a single forwarding rule line for DumpRules output.
type forwardEntry struct {
	ifaceIn  string
	ifaceOut string
	comment  string
}

// expandForwardEntries expands a Rule into one or more forward entries.
func expandForwardEntries(r Rule) []forwardEntry {
	switch r.Kind {
	case RuleInterPeerForward:
		return []forwardEntry{{
			ifaceIn:  r.Iface,
			ifaceOut: r.Iface,
			comment:  "inter-peer forwarding",
		}}
	case RuleBridgeForward:
		switch r.Direction {
		case "a_to_b":
			return []forwardEntry{{
				ifaceIn:  r.Iface,
				ifaceOut: r.IfaceB,
				comment:  fmt.Sprintf("bridge %s -> %s", r.Iface, r.IfaceB),
			}}
		case "b_to_a":
			return []forwardEntry{{
				ifaceIn:  r.IfaceB,
				ifaceOut: r.Iface,
				comment:  fmt.Sprintf("bridge %s -> %s", r.IfaceB, r.Iface),
			}}
		case "bidirectional":
			return []forwardEntry{
				{
					ifaceIn:  r.Iface,
					ifaceOut: r.IfaceB,
					comment:  fmt.Sprintf("bridge %s <-> %s", r.Iface, r.IfaceB),
				},
				{
					ifaceIn:  r.IfaceB,
					ifaceOut: r.Iface,
					comment:  fmt.Sprintf("bridge %s <-> %s", r.Iface, r.IfaceB),
				},
			}
		}
	}
	return nil
}

// formatRuleset formats rules into a human-readable nftables-style output.
func formatRuleset(rules map[string]Rule) string {
	if len(rules) == 0 {
		return "table ip wgpilot {\n}"
	}

	// Collect and sort rules by key for deterministic output.
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var natLines []string
	var fwdEntries []forwardEntry

	for _, k := range keys {
		r := rules[k]
		switch r.Kind {
		case RuleNATMasquerade:
			natLines = append(natLines, fmt.Sprintf(
				"    iifname %q oifname != %q masquerade  # NAT for %s",
				r.Iface, r.Iface, r.Subnet))
		case RuleInterPeerForward, RuleBridgeForward:
			fwdEntries = append(fwdEntries, expandForwardEntries(r)...)
		}
	}

	// Sort forward entries for deterministic output.
	sort.Slice(fwdEntries, func(i, j int) bool {
		if fwdEntries[i].ifaceIn != fwdEntries[j].ifaceIn {
			return fwdEntries[i].ifaceIn < fwdEntries[j].ifaceIn
		}
		return fwdEntries[i].ifaceOut < fwdEntries[j].ifaceOut
	})

	var b strings.Builder
	b.WriteString("table ip wgpilot {\n")

	if len(natLines) > 0 {
		b.WriteString("  chain postrouting {\n")
		b.WriteString("    type nat hook postrouting priority 100;\n")
		for _, line := range natLines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteString("  }\n")
	}

	if len(fwdEntries) > 0 {
		b.WriteString("  chain forward {\n")
		b.WriteString("    type filter hook forward priority 0;\n")
		for _, e := range fwdEntries {
			fmt.Fprintf(&b, "    iifname %q oifname %q accept  # %s\n",
				e.ifaceIn, e.ifaceOut, e.comment)
		}
		b.WriteString("  }\n")
	}

	b.WriteByte('}')
	return b.String()
}
