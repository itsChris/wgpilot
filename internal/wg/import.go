package wg

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ImportedConfig is the result of parsing a wg-quick .conf file.
type ImportedConfig struct {
	// Interface section
	PrivateKey  string
	Address     string // CIDR
	ListenPort  int
	DNSServers  string
	MTU         int

	// Peer sections
	Peers []ImportedPeer
}

// ImportedPeer is a single [Peer] section from a wg-quick .conf file.
type ImportedPeer struct {
	PublicKey           string
	PresharedKey        string
	AllowedIPs          string
	Endpoint            string
	PersistentKeepalive int
}

// ParseWgQuickConfig parses a wg-quick style INI configuration from the reader.
func ParseWgQuickConfig(r io.Reader) (*ImportedConfig, error) {
	scanner := bufio.NewScanner(r)
	cfg := &ImportedConfig{}

	var section string // "interface" or "peer"
	var currentPeer *ImportedPeer

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section headers.
		lower := strings.ToLower(line)
		if lower == "[interface]" {
			section = "interface"
			continue
		}
		if lower == "[peer]" {
			if currentPeer != nil {
				cfg.Peers = append(cfg.Peers, *currentPeer)
			}
			currentPeer = &ImportedPeer{}
			section = "peer"
			continue
		}

		// Key = Value pairs.
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch section {
		case "interface":
			switch key {
			case "privatekey":
				cfg.PrivateKey = value
			case "address":
				cfg.Address = value
			case "listenport":
				if n, err := strconv.Atoi(value); err == nil {
					cfg.ListenPort = n
				}
			case "dns":
				cfg.DNSServers = value
			case "mtu":
				if n, err := strconv.Atoi(value); err == nil {
					cfg.MTU = n
				}
			}
		case "peer":
			if currentPeer == nil {
				continue
			}
			switch key {
			case "publickey":
				currentPeer.PublicKey = value
			case "presharedkey":
				currentPeer.PresharedKey = value
			case "allowedips":
				currentPeer.AllowedIPs = value
			case "endpoint":
				currentPeer.Endpoint = value
			case "persistentkeepalive":
				if n, err := strconv.Atoi(value); err == nil {
					currentPeer.PersistentKeepalive = n
				}
			}
		}
	}

	// Don't forget the last peer.
	if currentPeer != nil {
		cfg.Peers = append(cfg.Peers, *currentPeer)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse wg-quick config: %w", err)
	}

	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("parse wg-quick config: no PrivateKey in [Interface] section")
	}

	return cfg, nil
}
