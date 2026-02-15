//go:build linux

package wg

import (
	"fmt"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// wgctrlClient implements WireGuardController using wgctrl.
type wgctrlClient struct {
	client *wgctrl.Client
}

// NewWireGuardController creates a new WireGuardController backed by wgctrl.
func NewWireGuardController() (WireGuardController, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("create wgctrl client: %w", err)
	}
	return &wgctrlClient{client: client}, nil
}

func (c *wgctrlClient) ConfigureDevice(name string, cfg DeviceConfig) error {
	wgCfg, err := toWGConfig(cfg)
	if err != nil {
		return fmt.Errorf("convert config for %s: %w", name, err)
	}
	return c.client.ConfigureDevice(name, wgCfg)
}

func (c *wgctrlClient) Device(name string) (*DeviceInfo, error) {
	dev, err := c.client.Device(name)
	if err != nil {
		return nil, err
	}
	return fromWGDevice(dev), nil
}

func (c *wgctrlClient) Devices() ([]*DeviceInfo, error) {
	devices, err := c.client.Devices()
	if err != nil {
		return nil, err
	}
	result := make([]*DeviceInfo, len(devices))
	for i, d := range devices {
		result[i] = fromWGDevice(d)
	}
	return result, nil
}

func (c *wgctrlClient) Close() error {
	return c.client.Close()
}

// toWGConfig converts DeviceConfig to wgtypes.Config.
func toWGConfig(cfg DeviceConfig) (wgtypes.Config, error) {
	var wgCfg wgtypes.Config

	if cfg.PrivateKey != "" {
		key, err := wgtypes.ParseKey(cfg.PrivateKey)
		if err != nil {
			return wgCfg, fmt.Errorf("parse private key: %w", err)
		}
		wgCfg.PrivateKey = &key
	}

	if cfg.ListenPort > 0 {
		wgCfg.ListenPort = &cfg.ListenPort
	}

	wgCfg.ReplacePeers = cfg.ReplacePeers

	for _, p := range cfg.Peers {
		peer, err := toWGPeerConfig(p)
		if err != nil {
			return wgCfg, err
		}
		wgCfg.Peers = append(wgCfg.Peers, peer)
	}

	return wgCfg, nil
}

// toWGPeerConfig converts WGPeerConfig to wgtypes.PeerConfig.
func toWGPeerConfig(p WGPeerConfig) (wgtypes.PeerConfig, error) {
	pubKey, err := wgtypes.ParseKey(p.PublicKey)
	if err != nil {
		return wgtypes.PeerConfig{}, fmt.Errorf("parse public key: %w", err)
	}

	peer := wgtypes.PeerConfig{
		PublicKey:         pubKey,
		Remove:            p.Remove,
		UpdateOnly:        p.UpdateOnly,
		ReplaceAllowedIPs: p.ReplaceAllowedIPs,
		AllowedIPs:        p.AllowedIPs,
	}

	if p.PresharedKey != "" {
		psk, err := wgtypes.ParseKey(p.PresharedKey)
		if err != nil {
			return wgtypes.PeerConfig{}, fmt.Errorf("parse preshared key: %w", err)
		}
		peer.PresharedKey = &psk
	}

	if p.Endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", p.Endpoint)
		if err != nil {
			return wgtypes.PeerConfig{}, fmt.Errorf("resolve endpoint %s: %w", p.Endpoint, err)
		}
		peer.Endpoint = addr
	}

	if p.PersistentKeepaliveInterval > 0 {
		d := p.PersistentKeepaliveInterval
		peer.PersistentKeepaliveInterval = &d
	}

	return peer, nil
}

// fromWGDevice converts wgtypes.Device to DeviceInfo.
func fromWGDevice(dev *wgtypes.Device) *DeviceInfo {
	info := &DeviceInfo{
		Name:       dev.Name,
		PublicKey:  dev.PublicKey.String(),
		ListenPort: dev.ListenPort,
	}

	for _, p := range dev.Peers {
		info.Peers = append(info.Peers, fromWGPeer(p))
	}

	return info
}

// fromWGPeer converts wgtypes.Peer to WGPeerInfo.
func fromWGPeer(p wgtypes.Peer) WGPeerInfo {
	var zeroKey wgtypes.Key

	info := WGPeerInfo{
		PublicKey:     p.PublicKey.String(),
		AllowedIPs:    p.AllowedIPs,
		LastHandshake: p.LastHandshakeTime,
		ReceiveBytes:  p.ReceiveBytes,
		TransmitBytes: p.TransmitBytes,
	}

	if p.PresharedKey != zeroKey {
		info.PresharedKey = p.PresharedKey.String()
	}

	if p.Endpoint != nil {
		info.Endpoint = p.Endpoint.String()
	}

	// Convert keepalive for logging
	_ = p.PersistentKeepaliveInterval

	return info
}

// wgConfigureTimeout returns the default timeout for WireGuard configuration operations.
func wgConfigureTimeout() time.Duration {
	return 10 * time.Second
}
