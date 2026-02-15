package wg

import (
	"net"
	"testing"
)

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return ipNet
}

func TestIPAllocator_Sequential(t *testing.T) {
	subnet := mustParseCIDR("10.0.0.0/24")
	alloc, err := NewIPAllocator(subnet, nil)
	if err != nil {
		t.Fatal(err)
	}

	// First allocation should be .2 (skipping .0 network and .1 server)
	ip1, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip1.String() != "10.0.0.2" {
		t.Errorf("first allocation: expected 10.0.0.2, got %s", ip1)
	}

	// Second allocation should be .3
	ip2, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip2.String() != "10.0.0.3" {
		t.Errorf("second allocation: expected 10.0.0.3, got %s", ip2)
	}

	// Third allocation should be .4
	ip3, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip3.String() != "10.0.0.4" {
		t.Errorf("third allocation: expected 10.0.0.4, got %s", ip3)
	}
}

func TestIPAllocator_SkipsServerIP(t *testing.T) {
	subnet := mustParseCIDR("10.0.0.0/24")
	alloc, err := NewIPAllocator(subnet, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Server IP is .1
	serverIP := alloc.ServerIP()
	if serverIP.String() != "10.0.0.1" {
		t.Errorf("expected server IP 10.0.0.1, got %s", serverIP)
	}

	// First allocation should skip .1
	ip, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() == "10.0.0.1" {
		t.Error("allocator should never return server IP .1")
	}
}

func TestIPAllocator_SkipsBroadcast(t *testing.T) {
	// /30 subnet: 10.0.0.0/30 has IPs .0 (net), .1 (server), .2, .3 (broadcast)
	subnet := mustParseCIDR("10.0.0.0/30")
	alloc, err := NewIPAllocator(subnet, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Only .2 should be allocatable (.0 is network, .1 is server, .3 is broadcast)
	ip, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2, got %s", ip)
	}

	// Next allocation should fail (only .3 left and it's broadcast)
	_, err = alloc.Allocate()
	if err == nil {
		t.Error("expected exhaustion error")
	}
}

func TestIPAllocator_Exhaustion(t *testing.T) {
	// /29 subnet: 10.0.0.0/29 has 8 IPs (0-7)
	// .0 = network, .1 = server, .7 = broadcast
	// Available: .2, .3, .4, .5, .6 = 5 addresses
	subnet := mustParseCIDR("10.0.0.0/29")
	alloc, err := NewIPAllocator(subnet, nil)
	if err != nil {
		t.Fatal(err)
	}

	var allocated []net.IP
	for i := 0; i < 5; i++ {
		ip, err := alloc.Allocate()
		if err != nil {
			t.Fatalf("allocation %d failed unexpectedly: %v", i, err)
		}
		allocated = append(allocated, ip)
	}

	// Verify we got .2 through .6
	expected := []string{"10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5", "10.0.0.6"}
	for i, exp := range expected {
		if allocated[i].String() != exp {
			t.Errorf("allocation %d: expected %s, got %s", i, exp, allocated[i])
		}
	}

	// Next allocation should fail
	_, err = alloc.Allocate()
	if err == nil {
		t.Error("expected exhaustion error, got nil")
	}
}

func TestIPAllocator_Release(t *testing.T) {
	subnet := mustParseCIDR("10.0.0.0/30")
	alloc, err := NewIPAllocator(subnet, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate the only available IP
	ip, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %s", ip)
	}

	// Should be exhausted
	_, err = alloc.Allocate()
	if err == nil {
		t.Error("expected exhaustion")
	}

	// Release the IP
	alloc.Release(ip)

	// Should be able to allocate again
	ip2, err := alloc.Allocate()
	if err != nil {
		t.Fatalf("expected allocation after release, got error: %v", err)
	}
	if ip2.String() != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2 after release, got %s", ip2)
	}
}

func TestIPAllocator_WithExistingAllocations(t *testing.T) {
	subnet := mustParseCIDR("10.0.0.0/24")
	existing := []net.IP{
		net.ParseIP("10.0.0.2"),
		net.ParseIP("10.0.0.3"),
		net.ParseIP("10.0.0.5"),
	}
	alloc, err := NewIPAllocator(subnet, existing)
	if err != nil {
		t.Fatal(err)
	}

	// First allocation should skip .2 and .3, return .4
	ip, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "10.0.0.4" {
		t.Errorf("expected 10.0.0.4 (skipping existing), got %s", ip)
	}

	// Next should skip .5, return .6
	ip, err = alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "10.0.0.6" {
		t.Errorf("expected 10.0.0.6 (skipping .5), got %s", ip)
	}
}

func TestIPAllocator_NilSubnet(t *testing.T) {
	_, err := NewIPAllocator(nil, nil)
	if err == nil {
		t.Error("expected error for nil subnet")
	}
}

func TestIPAllocator_Used(t *testing.T) {
	subnet := mustParseCIDR("10.0.0.0/24")
	alloc, err := NewIPAllocator(subnet, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Initially: .0 (network) + .1 (server) = 2
	if alloc.Used() != 2 {
		t.Errorf("expected 2 reserved, got %d", alloc.Used())
	}

	_, _ = alloc.Allocate()
	if alloc.Used() != 3 {
		t.Errorf("expected 3 after one allocation, got %d", alloc.Used())
	}
}

func TestFirstUsableIP(t *testing.T) {
	tests := []struct {
		subnet   string
		expected string
	}{
		{"10.0.0.0/24", "10.0.0.1"},
		{"192.168.1.0/24", "192.168.1.1"},
		{"172.16.0.0/16", "172.16.0.1"},
		{"10.0.0.0/30", "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.subnet, func(t *testing.T) {
			subnet := mustParseCIDR(tt.subnet)
			got := firstUsableIP(subnet)
			if got.String() != tt.expected {
				t.Errorf("firstUsableIP(%s): expected %s, got %s", tt.subnet, tt.expected, got)
			}
		})
	}
}

func TestIsBroadcast(t *testing.T) {
	subnet := mustParseCIDR("10.0.0.0/24")

	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.0", false},
		{"10.0.0.1", false},
		{"10.0.0.254", false},
		{"10.0.0.255", true},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if isBroadcast(ip, subnet) != tt.expected {
				t.Errorf("isBroadcast(%s, %s): expected %v", tt.ip, subnet, tt.expected)
			}
		})
	}
}
