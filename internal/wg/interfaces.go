package wg

import (
	"context"
	"net"
	"time"
)

// WireGuardController abstracts WireGuard kernel operations for testability.
// The real implementation wraps wgctrl.Client.
type WireGuardController interface {
	// ConfigureDevice applies configuration to a WireGuard device.
	ConfigureDevice(name string, cfg DeviceConfig) error

	// Device returns information about a WireGuard device.
	Device(name string) (*DeviceInfo, error)

	// Devices returns all WireGuard devices.
	Devices() ([]*DeviceInfo, error)

	// Close releases resources.
	Close() error
}

// LinkManager abstracts network interface management for testability.
// The real implementation wraps vishvananda/netlink.
type LinkManager interface {
	// CreateWireGuardLink creates a new WireGuard network interface.
	CreateWireGuardLink(name string) error

	// DeleteLink removes a network interface.
	DeleteLink(name string) error

	// SetLinkUp brings a network interface up.
	SetLinkUp(name string) error

	// SetLinkDown takes a network interface down.
	SetLinkDown(name string) error

	// AddAddress assigns a CIDR address to a network interface.
	AddAddress(linkName string, addr string) error

	// ListAddresses returns CIDR addresses assigned to a network interface.
	ListAddresses(linkName string) ([]string, error)

	// LinkExists checks if a network interface with the given name exists.
	LinkExists(name string) (bool, error)
}

// NetworkStore provides read access to network and peer data for reconciliation.
type NetworkStore interface {
	ListNetworks(ctx context.Context) ([]NetworkConfig, error)
	ListPeersByNetworkID(ctx context.Context, networkID int64) ([]PeerConfig, error)
	ListBridges(ctx context.Context) ([]BridgeConfig, error)
}

// BridgeConfig contains the fields needed to reconcile bridge nftables rules.
type BridgeConfig struct {
	ID         int64
	InterfaceA string
	InterfaceB string
	Direction  string
	Enabled    bool
}

// DeviceConfig holds configuration to apply to a WireGuard device.
type DeviceConfig struct {
	PrivateKey   string
	ListenPort   int
	ReplacePeers bool
	Peers        []WGPeerConfig
}

// WGPeerConfig holds configuration for a single WireGuard peer.
type WGPeerConfig struct {
	PublicKey                   string
	Remove                     bool
	UpdateOnly                 bool
	PresharedKey                string
	Endpoint                   string
	PersistentKeepaliveInterval time.Duration
	ReplaceAllowedIPs          bool
	AllowedIPs                 []net.IPNet
}

// DeviceInfo holds runtime information about a WireGuard device.
type DeviceInfo struct {
	Name       string
	PublicKey  string
	ListenPort int
	Peers      []WGPeerInfo
}

// WGPeerInfo holds runtime information about a single WireGuard peer.
type WGPeerInfo struct {
	PublicKey     string
	PresharedKey  string
	Endpoint      string
	AllowedIPs    []net.IPNet
	LastHandshake time.Time
	ReceiveBytes  int64
	TransmitBytes int64
}

// NetworkConfig contains the fields needed to create/manage a WireGuard interface.
type NetworkConfig struct {
	ID               int64
	Name             string
	Interface        string
	Mode             string
	Subnet           string
	ListenPort       int
	PrivateKey       string
	PublicKey        string
	DNSServers       string
	NATEnabled       bool
	InterPeerRouting bool
	Enabled          bool
}

// PeerConfig contains the fields needed to configure a WireGuard peer.
type PeerConfig struct {
	ID                  int64
	NetworkID           int64
	Name                string
	PublicKey           string
	PresharedKey        string
	AllowedIPs          string
	Endpoint            string
	PersistentKeepalive int
	Role                string
	SiteNetworks        string
	Enabled             bool
}

// PeerStatus represents the runtime status of a peer from the kernel.
type PeerStatus struct {
	PublicKey     string
	Endpoint      string
	LastHandshake time.Time
	TransferRx    int64
	TransferTx    int64
	AllowedIPs    []string
	Online        bool
}

// ClientConfigParams holds all parameters needed to generate a client .conf file.
type ClientConfigParams struct {
	PeerName            string
	PeerPrivateKey      string
	PeerAddress         string
	DNSServers          string
	ServerPublicKey     string
	PresharedKey        string
	ServerEndpoint      string
	AllowedIPs          string
	PersistentKeepalive int
}
