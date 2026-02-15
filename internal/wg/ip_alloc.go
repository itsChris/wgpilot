package wg

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

// IPAllocator manages IP address allocation from a subnet.
// The network address and .1 (server) are always reserved. Broadcast is excluded.
type IPAllocator struct {
	mu     sync.Mutex
	subnet *net.IPNet
	used   map[string]bool
}

// NewIPAllocator creates an allocator for the given subnet with pre-existing allocations.
func NewIPAllocator(subnet *net.IPNet, used []net.IP) (*IPAllocator, error) {
	if subnet == nil {
		return nil, fmt.Errorf("new ip allocator: subnet is nil")
	}

	a := &IPAllocator{
		subnet: &net.IPNet{
			IP:   make(net.IP, len(subnet.IP)),
			Mask: make(net.IPMask, len(subnet.Mask)),
		},
		used: make(map[string]bool),
	}
	copy(a.subnet.IP, subnet.IP)
	copy(a.subnet.Mask, subnet.Mask)

	// Normalize subnet IP to network address
	a.subnet.IP = a.subnet.IP.Mask(a.subnet.Mask)

	// Reserve network address
	a.used[a.subnet.IP.String()] = true

	// Reserve server address (.1)
	serverIP := firstUsableIP(a.subnet)
	a.used[serverIP.String()] = true

	// Mark pre-existing allocations
	for _, ip := range used {
		a.used[ip.String()] = true
	}

	return a, nil
}

// Allocate returns the next available IP from the subnet.
func (a *IPAllocator) Allocate() (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	serverIP := firstUsableIP(a.subnet)
	for ip := nextIP(serverIP); a.subnet.Contains(ip); ip = nextIP(ip) {
		if isBroadcast(ip, a.subnet) {
			continue
		}
		if !a.used[ip.String()] {
			allocated := make(net.IP, len(ip))
			copy(allocated, ip)
			a.used[allocated.String()] = true
			return allocated, nil
		}
	}

	return nil, fmt.Errorf("allocate ip: no available IPs in subnet %s", a.subnet.String())
}

// Release returns an IP to the pool for re-use.
func (a *IPAllocator) Release(ip net.IP) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, ip.String())
}

// Used returns the count of allocated IPs (including reserved).
func (a *IPAllocator) Used() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.used)
}

// ServerIP returns the server address (first usable IP, typically .1).
func (a *IPAllocator) ServerIP() net.IP {
	return firstUsableIP(a.subnet)
}

// firstUsableIP returns the first usable host IP in the subnet (network + 1).
func firstUsableIP(subnet *net.IPNet) net.IP {
	networkIP := subnet.IP.Mask(subnet.Mask)
	ip := make(net.IP, len(networkIP))
	copy(ip, networkIP)
	return nextIP(ip)
}

// nextIP increments an IP address by one.
func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)

	// Work with 4-byte representation for IPv4
	ip4 := next.To4()
	if ip4 != nil {
		n := binary.BigEndian.Uint32(ip4)
		n++
		binary.BigEndian.PutUint32(ip4, n)
		return ip4
	}

	// IPv6: increment as 16-byte big-endian integer
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

// isBroadcast checks if an IP is the broadcast address of the subnet.
func isBroadcast(ip net.IP, subnet *net.IPNet) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	mask := subnet.Mask
	if len(mask) == 16 {
		mask = mask[12:]
	}

	network := subnet.IP.To4()
	if network == nil {
		return false
	}
	network = network.Mask(mask)

	for i := range ip4 {
		if ip4[i] != (network[i] | ^mask[i]) {
			return false
		}
	}
	return true
}
