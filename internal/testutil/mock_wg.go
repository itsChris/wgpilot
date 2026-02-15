package testutil

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/itsChris/wgpilot/internal/wg"
)

// MockCall records a method invocation with its arguments.
type MockCall struct {
	Method string
	Args   []any
}

// MockWireGuardController implements wg.WireGuardController for testing.
type MockWireGuardController struct {
	mu    sync.Mutex
	Calls []MockCall

	ConfigureDeviceFn func(name string, cfg wg.DeviceConfig) error
	DeviceFn          func(name string) (*wg.DeviceInfo, error)
	DevicesFn         func() ([]*wg.DeviceInfo, error)
	CloseFn           func() error
}

func (m *MockWireGuardController) ConfigureDevice(name string, cfg wg.DeviceConfig) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "ConfigureDevice", Args: []any{name, cfg}})
	m.mu.Unlock()
	if m.ConfigureDeviceFn != nil {
		return m.ConfigureDeviceFn(name, cfg)
	}
	return nil
}

func (m *MockWireGuardController) Device(name string) (*wg.DeviceInfo, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "Device", Args: []any{name}})
	m.mu.Unlock()
	if m.DeviceFn != nil {
		return m.DeviceFn(name)
	}
	return &wg.DeviceInfo{Name: name}, nil
}

func (m *MockWireGuardController) Devices() ([]*wg.DeviceInfo, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "Devices"})
	m.mu.Unlock()
	if m.DevicesFn != nil {
		return m.DevicesFn()
	}
	return nil, nil
}

func (m *MockWireGuardController) Close() error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "Close"})
	m.mu.Unlock()
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

// CallMethods returns the method names of all recorded calls.
func (m *MockWireGuardController) CallMethods() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	methods := make([]string, len(m.Calls))
	for i, c := range m.Calls {
		methods[i] = c.Method
	}
	return methods
}

// MockLinkManager implements wg.LinkManager for testing.
type MockLinkManager struct {
	mu    sync.Mutex
	Calls []MockCall

	CreateWireGuardLinkFn func(name string) error
	DeleteLinkFn          func(name string) error
	SetLinkUpFn           func(name string) error
	SetLinkDownFn         func(name string) error
	AddAddressFn          func(linkName string, addr string) error
	ListAddressesFn       func(linkName string) ([]string, error)
	LinkExistsFn          func(name string) (bool, error)
}

func (m *MockLinkManager) CreateWireGuardLink(name string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "CreateWireGuardLink", Args: []any{name}})
	m.mu.Unlock()
	if m.CreateWireGuardLinkFn != nil {
		return m.CreateWireGuardLinkFn(name)
	}
	return nil
}

func (m *MockLinkManager) DeleteLink(name string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "DeleteLink", Args: []any{name}})
	m.mu.Unlock()
	if m.DeleteLinkFn != nil {
		return m.DeleteLinkFn(name)
	}
	return nil
}

func (m *MockLinkManager) SetLinkUp(name string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "SetLinkUp", Args: []any{name}})
	m.mu.Unlock()
	if m.SetLinkUpFn != nil {
		return m.SetLinkUpFn(name)
	}
	return nil
}

func (m *MockLinkManager) SetLinkDown(name string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "SetLinkDown", Args: []any{name}})
	m.mu.Unlock()
	if m.SetLinkDownFn != nil {
		return m.SetLinkDownFn(name)
	}
	return nil
}

func (m *MockLinkManager) AddAddress(linkName string, addr string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "AddAddress", Args: []any{linkName, addr}})
	m.mu.Unlock()
	if m.AddAddressFn != nil {
		return m.AddAddressFn(linkName, addr)
	}
	return nil
}

func (m *MockLinkManager) ListAddresses(linkName string) ([]string, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "ListAddresses", Args: []any{linkName}})
	m.mu.Unlock()
	if m.ListAddressesFn != nil {
		return m.ListAddressesFn(linkName)
	}
	return nil, nil
}

func (m *MockLinkManager) LinkExists(name string) (bool, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Method: "LinkExists", Args: []any{name}})
	m.mu.Unlock()
	if m.LinkExistsFn != nil {
		return m.LinkExistsFn(name)
	}
	return false, nil
}

// CallMethods returns the method names of all recorded calls.
func (m *MockLinkManager) CallMethods() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	methods := make([]string, len(m.Calls))
	for i, c := range m.Calls {
		methods[i] = c.Method
	}
	return methods
}

// MockNetworkStore implements wg.NetworkStore for testing.
type MockNetworkStore struct {
	ListNetworksFn         func(ctx context.Context) ([]wg.NetworkConfig, error)
	ListPeersByNetworkIDFn func(ctx context.Context, networkID int64) ([]wg.PeerConfig, error)
}

func (m *MockNetworkStore) ListNetworks(ctx context.Context) ([]wg.NetworkConfig, error) {
	if m.ListNetworksFn != nil {
		return m.ListNetworksFn(ctx)
	}
	return nil, nil
}

func (m *MockNetworkStore) ListPeersByNetworkID(ctx context.Context, networkID int64) ([]wg.PeerConfig, error) {
	if m.ListPeersByNetworkIDFn != nil {
		return m.ListPeersByNetworkIDFn(ctx, networkID)
	}
	return nil, nil
}

// MustParseCIDR parses a CIDR string and panics on failure. For tests only.
func MustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %q: %v", s, err))
	}
	return ipNet
}
