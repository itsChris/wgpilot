package nft

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// failApplier is an Applier that always returns an error.
type failApplier struct{}

func (failApplier) Apply([]Rule) error { return errors.New("apply failed") }

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewTestManager(testLogger(), false)
	if err != nil {
		t.Fatalf("NewTestManager: %v", err)
	}
	return m
}

func newDevTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewTestManager(testLogger(), true)
	if err != nil {
		t.Fatalf("NewTestManager: %v", err)
	}
	return m
}

// --- Constructor Tests ---

func TestNewManager_NilApplier(t *testing.T) {
	_, err := NewManager(nil, testLogger(), false)
	if err == nil {
		t.Fatal("expected error for nil applier")
	}
	if !strings.Contains(err.Error(), "applier is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewManager_NilLogger(t *testing.T) {
	_, err := NewManager(noopApplier{}, nil, false)
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
	if !strings.Contains(err.Error(), "logger is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- NAT Masquerade Tests ---

func TestAddNATMasquerade_Success(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatalf("AddNATMasquerade: %v", err)
	}

	dump, _ := m.DumpRules()
	if !strings.Contains(dump, `iifname "wg0"`) {
		t.Errorf("dump missing wg0 iifname:\n%s", dump)
	}
	if !strings.Contains(dump, "masquerade") {
		t.Errorf("dump missing masquerade:\n%s", dump)
	}
	if !strings.Contains(dump, "10.0.0.0/24") {
		t.Errorf("dump missing subnet:\n%s", dump)
	}
}

func TestAddNATMasquerade_MultipleInterfaces(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddNATMasquerade("wg1", "10.1.0.0/24"); err != nil {
		t.Fatal(err)
	}

	dump, _ := m.DumpRules()
	if !strings.Contains(dump, `iifname "wg0"`) || !strings.Contains(dump, `iifname "wg1"`) {
		t.Errorf("dump missing both interfaces:\n%s", dump)
	}
}

func TestAddNATMasquerade_Idempotent(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}

	dump, _ := m.DumpRules()
	count := strings.Count(dump, "masquerade")
	if count != 1 {
		t.Errorf("expected 1 masquerade rule, found %d:\n%s", count, dump)
	}
}

func TestAddNATMasquerade_ApplyError(t *testing.T) {
	m, err := NewManager(failApplier{}, testLogger(), false)
	if err != nil {
		t.Fatal(err)
	}

	err = m.AddNATMasquerade("wg0", "10.0.0.0/24")
	if err == nil {
		t.Fatal("expected error from failing applier")
	}

	// Rule should be rolled back.
	m.mu.Lock()
	ruleCount := len(m.rules)
	m.mu.Unlock()
	if ruleCount != 0 {
		t.Errorf("expected 0 rules after rollback, got %d", ruleCount)
	}
}

func TestRemoveNATMasquerade_Success(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveNATMasquerade("wg0"); err != nil {
		t.Fatalf("RemoveNATMasquerade: %v", err)
	}

	dump, _ := m.DumpRules()
	if strings.Contains(dump, "masquerade") {
		t.Errorf("dump should not contain masquerade after removal:\n%s", dump)
	}
}

func TestRemoveNATMasquerade_NotFound(t *testing.T) {
	m := newTestManager(t)

	if err := m.RemoveNATMasquerade("wg99"); err != nil {
		t.Errorf("RemoveNATMasquerade for non-existent should return nil, got: %v", err)
	}
}

func TestRemoveNATMasquerade_ApplyError(t *testing.T) {
	m := newTestManager(t)
	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}

	// Switch to failing applier.
	m.mu.Lock()
	m.applier = failApplier{}
	m.mu.Unlock()

	if err := m.RemoveNATMasquerade("wg0"); err == nil {
		t.Fatal("expected error from failing applier")
	}

	// Rule should be restored.
	m.mu.Lock()
	ruleCount := len(m.rules)
	m.mu.Unlock()
	if ruleCount != 1 {
		t.Errorf("expected 1 rule after rollback, got %d", ruleCount)
	}
}

// --- Inter-Peer Forwarding Tests ---

func TestEnableInterPeerForwarding_Success(t *testing.T) {
	m := newTestManager(t)

	if err := m.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatalf("EnableInterPeerForwarding: %v", err)
	}

	dump, _ := m.DumpRules()
	if !strings.Contains(dump, `iifname "wg0" oifname "wg0" accept`) {
		t.Errorf("dump missing inter-peer forwarding rule:\n%s", dump)
	}
	if !strings.Contains(dump, "inter-peer forwarding") {
		t.Errorf("dump missing comment:\n%s", dump)
	}
}

func TestEnableInterPeerForwarding_Idempotent(t *testing.T) {
	m := newTestManager(t)

	if err := m.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}

	dump, _ := m.DumpRules()
	count := strings.Count(dump, "inter-peer forwarding")
	if count != 1 {
		t.Errorf("expected 1 inter-peer forwarding rule, found %d:\n%s", count, dump)
	}
}

func TestEnableInterPeerForwarding_ApplyError(t *testing.T) {
	m, err := NewManager(failApplier{}, testLogger(), false)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.EnableInterPeerForwarding("wg0"); err == nil {
		t.Fatal("expected error from failing applier")
	}

	m.mu.Lock()
	ruleCount := len(m.rules)
	m.mu.Unlock()
	if ruleCount != 0 {
		t.Errorf("expected 0 rules after rollback, got %d", ruleCount)
	}
}

func TestDisableInterPeerForwarding_Success(t *testing.T) {
	m := newTestManager(t)

	if err := m.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableInterPeerForwarding("wg0"); err != nil {
		t.Fatalf("DisableInterPeerForwarding: %v", err)
	}

	dump, _ := m.DumpRules()
	if strings.Contains(dump, "inter-peer") {
		t.Errorf("dump should not contain inter-peer rule after disable:\n%s", dump)
	}
}

func TestDisableInterPeerForwarding_NotFound(t *testing.T) {
	m := newTestManager(t)

	if err := m.DisableInterPeerForwarding("wg99"); err != nil {
		t.Errorf("DisableInterPeerForwarding for non-existent should return nil, got: %v", err)
	}
}

// --- Network Bridge Tests ---

func TestAddNetworkBridge_AToB(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNetworkBridge("wg0", "wg1", "a_to_b"); err != nil {
		t.Fatalf("AddNetworkBridge: %v", err)
	}

	dump, _ := m.DumpRules()
	if !strings.Contains(dump, `iifname "wg0" oifname "wg1" accept`) {
		t.Errorf("dump missing a->b forwarding:\n%s", dump)
	}
	// Should NOT contain reverse direction.
	if strings.Contains(dump, `iifname "wg1" oifname "wg0" accept`) {
		t.Errorf("dump should not contain b->a for a_to_b direction:\n%s", dump)
	}
}

func TestAddNetworkBridge_BToA(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNetworkBridge("wg0", "wg1", "b_to_a"); err != nil {
		t.Fatalf("AddNetworkBridge: %v", err)
	}

	dump, _ := m.DumpRules()
	if !strings.Contains(dump, `iifname "wg1" oifname "wg0" accept`) {
		t.Errorf("dump missing b->a forwarding:\n%s", dump)
	}
	// Should NOT contain forward direction.
	if strings.Contains(dump, `iifname "wg0" oifname "wg1" accept`) {
		t.Errorf("dump should not contain a->b for b_to_a direction:\n%s", dump)
	}
}

func TestAddNetworkBridge_Bidirectional(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNetworkBridge("wg0", "wg1", "bidirectional"); err != nil {
		t.Fatalf("AddNetworkBridge: %v", err)
	}

	dump, _ := m.DumpRules()
	if !strings.Contains(dump, `iifname "wg0" oifname "wg1" accept`) {
		t.Errorf("dump missing a->b forwarding:\n%s", dump)
	}
	if !strings.Contains(dump, `iifname "wg1" oifname "wg0" accept`) {
		t.Errorf("dump missing b->a forwarding:\n%s", dump)
	}
}

func TestAddNetworkBridge_InvalidDirection(t *testing.T) {
	m := newTestManager(t)

	err := m.AddNetworkBridge("wg0", "wg1", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
	if !strings.Contains(err.Error(), "invalid direction") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAddNetworkBridge_UpdateDirection(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNetworkBridge("wg0", "wg1", "a_to_b"); err != nil {
		t.Fatal(err)
	}

	// Verify only a->b exists.
	dump, _ := m.DumpRules()
	if strings.Contains(dump, `iifname "wg1" oifname "wg0" accept`) {
		t.Fatal("should not have b->a before update")
	}

	// Update to bidirectional.
	if err := m.AddNetworkBridge("wg0", "wg1", "bidirectional"); err != nil {
		t.Fatal(err)
	}

	dump, _ = m.DumpRules()
	if !strings.Contains(dump, `iifname "wg0" oifname "wg1" accept`) {
		t.Errorf("dump missing a->b after update:\n%s", dump)
	}
	if !strings.Contains(dump, `iifname "wg1" oifname "wg0" accept`) {
		t.Errorf("dump missing b->a after update:\n%s", dump)
	}
}

func TestAddNetworkBridge_ApplyError(t *testing.T) {
	m, err := NewManager(failApplier{}, testLogger(), false)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.AddNetworkBridge("wg0", "wg1", "bidirectional"); err == nil {
		t.Fatal("expected error from failing applier")
	}

	m.mu.Lock()
	ruleCount := len(m.rules)
	m.mu.Unlock()
	if ruleCount != 0 {
		t.Errorf("expected 0 rules after rollback, got %d", ruleCount)
	}
}

func TestRemoveNetworkBridge_Success(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNetworkBridge("wg0", "wg1", "bidirectional"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveNetworkBridge("wg0", "wg1"); err != nil {
		t.Fatalf("RemoveNetworkBridge: %v", err)
	}

	dump, _ := m.DumpRules()
	if strings.Contains(dump, "bridge") {
		t.Errorf("dump should not contain bridge rules after removal:\n%s", dump)
	}
}

func TestRemoveNetworkBridge_ReverseOrder(t *testing.T) {
	m := newTestManager(t)

	// Add with (wg0, wg1) but remove with (wg1, wg0).
	if err := m.AddNetworkBridge("wg0", "wg1", "a_to_b"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveNetworkBridge("wg1", "wg0"); err != nil {
		t.Fatalf("RemoveNetworkBridge reverse order: %v", err)
	}

	dump, _ := m.DumpRules()
	if strings.Contains(dump, "bridge") {
		t.Errorf("dump should not contain bridge rules after removal:\n%s", dump)
	}
}

func TestRemoveNetworkBridge_NotFound(t *testing.T) {
	m := newTestManager(t)

	if err := m.RemoveNetworkBridge("wg0", "wg1"); err != nil {
		t.Errorf("RemoveNetworkBridge for non-existent should return nil, got: %v", err)
	}
}

// --- DumpRules Tests ---

func TestDumpRules_Empty(t *testing.T) {
	m := newTestManager(t)

	dump, err := m.DumpRules()
	if err != nil {
		t.Fatalf("DumpRules: %v", err)
	}

	expected := "table ip wgpilot {\n}"
	if dump != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, dump)
	}
}

func TestDumpRules_MultipleTypes(t *testing.T) {
	m := newTestManager(t)

	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableInterPeerForwarding("wg1"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddNetworkBridge("wg0", "wg1", "a_to_b"); err != nil {
		t.Fatal(err)
	}

	dump, err := m.DumpRules()
	if err != nil {
		t.Fatalf("DumpRules: %v", err)
	}

	// Verify structure.
	if !strings.Contains(dump, "table ip wgpilot") {
		t.Error("dump missing table declaration")
	}
	if !strings.Contains(dump, "chain postrouting") {
		t.Error("dump missing postrouting chain")
	}
	if !strings.Contains(dump, "chain forward") {
		t.Error("dump missing forward chain")
	}
	if !strings.Contains(dump, "masquerade") {
		t.Error("dump missing masquerade rule")
	}
	if !strings.Contains(dump, "inter-peer forwarding") {
		t.Error("dump missing inter-peer forwarding rule")
	}
	if !strings.Contains(dump, "bridge wg0 -> wg1") {
		t.Error("dump missing bridge rule")
	}
}

func TestDumpRules_Deterministic(t *testing.T) {
	m := newTestManager(t)

	// Add rules in different orders and verify output is the same.
	if err := m.AddNATMasquerade("wg1", "10.1.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}

	dump1, _ := m.DumpRules()

	// Create a second manager with same rules in different order.
	m2 := newTestManager(t)
	if err := m2.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m2.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}
	if err := m2.AddNATMasquerade("wg1", "10.1.0.0/24"); err != nil {
		t.Fatal(err)
	}

	dump2, _ := m2.DumpRules()

	if dump1 != dump2 {
		t.Errorf("dumps should be identical regardless of insertion order:\n--- dump1 ---\n%s\n--- dump2 ---\n%s", dump1, dump2)
	}
}

// --- Dev Mode Tests ---

func TestDevMode_DumpsAfterChange(t *testing.T) {
	// Verify that dev mode doesn't panic during all operations.
	m := newDevTestManager(t)

	if err := m.AddNATMasquerade("wg0", "10.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	if err := m.EnableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddNetworkBridge("wg0", "wg1", "bidirectional"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveNATMasquerade("wg0"); err != nil {
		t.Fatal(err)
	}
	if err := m.DisableInterPeerForwarding("wg0"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveNetworkBridge("wg0", "wg1"); err != nil {
		t.Fatal(err)
	}
}

// --- Concurrent Access Test ---

func TestConcurrentAccess(t *testing.T) {
	m := newTestManager(t)

	var wg sync.WaitGroup

	// Phase 1: Concurrent adds.
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			iface := fmt.Sprintf("wg%d", i)
			_ = m.AddNATMasquerade(iface, fmt.Sprintf("10.%d.0.0/24", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			iface := fmt.Sprintf("wg%d", i)
			_ = m.EnableInterPeerForwarding(iface)
		}(i)
		go func(i int) {
			defer wg.Done()
			_, _ = m.DumpRules()
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := fmt.Sprintf("wg%d", i)
			b := fmt.Sprintf("wg%d", i+5)
			_ = m.AddNetworkBridge(a, b, "bidirectional")
		}(i)
	}
	wg.Wait()

	// Phase 2: Concurrent removes (all adds completed first).
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			iface := fmt.Sprintf("wg%d", i)
			_ = m.RemoveNATMasquerade(iface)
		}(i)
		go func(i int) {
			defer wg.Done()
			iface := fmt.Sprintf("wg%d", i)
			_ = m.DisableInterPeerForwarding(iface)
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := fmt.Sprintf("wg%d", i)
			b := fmt.Sprintf("wg%d", i+5)
			_ = m.RemoveNetworkBridge(a, b)
		}(i)
	}
	wg.Wait()

	// After all removes, rules should be empty.
	dump, _ := m.DumpRules()
	expected := "table ip wgpilot {\n}"
	if dump != expected {
		t.Errorf("expected empty ruleset after concurrent removes:\n%s", dump)
	}
}
