package nft

import (
	"fmt"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
)

const tableName = "wgpilot"

// nftApplier implements Applier using the google/nftables library.
type nftApplier struct{}

// NewApplier creates an Applier that uses the kernel nftables API.
// Requires CAP_NET_ADMIN capability.
func NewApplier() Applier {
	return &nftApplier{}
}

func (a *nftApplier) Apply(rules []Rule) error {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nftables connect: %w", err)
	}

	// Delete existing wgpilot table if it exists.
	tables, err := conn.ListTables()
	if err != nil {
		return fmt.Errorf("nftables list tables: %w", err)
	}
	for _, t := range tables {
		if t.Name == tableName && t.Family == nftables.TableFamilyIPv4 {
			conn.DelTable(t)
			if err := conn.Flush(); err != nil {
				return fmt.Errorf("nftables delete table: %w", err)
			}
			break
		}
	}

	if len(rules) == 0 {
		return nil
	}

	// Create fresh table.
	table := conn.AddTable(&nftables.Table{
		Name:   tableName,
		Family: nftables.TableFamilyIPv4,
	})

	// Separate rules by chain type.
	var natRules, forwardRules []Rule
	for _, r := range rules {
		switch r.Kind {
		case RuleNATMasquerade:
			natRules = append(natRules, r)
		case RuleInterPeerForward, RuleBridgeForward:
			forwardRules = append(forwardRules, r)
		}
	}

	// Postrouting chain for NAT masquerade.
	if len(natRules) > 0 {
		chain := conn.AddChain(&nftables.Chain{
			Name:     "postrouting",
			Table:    table,
			Type:     nftables.ChainTypeNAT,
			Hooknum:  nftables.ChainHookPostrouting,
			Priority: nftables.ChainPriorityNATSource,
		})
		for _, r := range natRules {
			conn.AddRule(&nftables.Rule{
				Table: table,
				Chain: chain,
				Exprs: masqueradeExprs(r.Iface),
			})
		}
	}

	// Forward chain for inter-peer and bridge rules.
	if len(forwardRules) > 0 {
		chain := conn.AddChain(&nftables.Chain{
			Name:     "forward",
			Table:    table,
			Type:     nftables.ChainTypeFilter,
			Hooknum:  nftables.ChainHookForward,
			Priority: nftables.ChainPriorityFilter,
		})
		for _, r := range forwardRules {
			for _, exprs := range buildForwardExprs(r) {
				conn.AddRule(&nftables.Rule{
					Table: table,
					Chain: chain,
					Exprs: exprs,
				})
			}
		}
	}

	if err := conn.Flush(); err != nil {
		return fmt.Errorf("nftables flush: %w", err)
	}
	return nil
}

// masqueradeExprs builds nftables expressions for:
//
//	iifname <iface> oifname != <iface> masquerade
func masqueradeExprs(iface string) []expr.Any {
	ifaceData := ifaceBytes(iface)
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifaceData},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: ifaceData},
		&expr.Masq{},
	}
}

// buildForwardExprs builds nftables expressions for forward rules.
// Returns one or two expression sets depending on bridge direction.
func buildForwardExprs(r Rule) [][]expr.Any {
	switch r.Kind {
	case RuleInterPeerForward:
		return [][]expr.Any{forwardPairExprs(r.Iface, r.Iface)}
	case RuleBridgeForward:
		switch r.Direction {
		case "a_to_b":
			return [][]expr.Any{forwardPairExprs(r.Iface, r.IfaceB)}
		case "b_to_a":
			return [][]expr.Any{forwardPairExprs(r.IfaceB, r.Iface)}
		case "bidirectional":
			return [][]expr.Any{
				forwardPairExprs(r.Iface, r.IfaceB),
				forwardPairExprs(r.IfaceB, r.Iface),
			}
		}
	}
	return nil
}

// forwardPairExprs builds expressions for:
//
//	iifname <in> oifname <out> accept
func forwardPairExprs(ifaceIn, ifaceOut string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifaceBytes(ifaceIn)},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifaceBytes(ifaceOut)},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

// ifaceBytes returns the interface name as a null-terminated byte slice
// for use in nftables comparison expressions.
func ifaceBytes(iface string) []byte {
	b := make([]byte, len(iface)+1)
	copy(b, iface)
	return b
}

// noopApplier is an Applier that does nothing. Used for testing.
type noopApplier struct{}

func (noopApplier) Apply([]Rule) error { return nil }
