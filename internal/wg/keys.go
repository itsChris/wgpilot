package wg

import (
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// GenerateKeyPair generates a new WireGuard private/public key pair.
// Keys are returned as base64-encoded strings.
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", "", fmt.Errorf("generate key pair: %w", err)
	}
	return key.String(), key.PublicKey().String(), nil
}

// GeneratePresharedKey generates a random preshared key for additional security.
// The key is returned as a base64-encoded string.
func GeneratePresharedKey() (string, error) {
	key, err := wgtypes.GenerateKey()
	if err != nil {
		return "", fmt.Errorf("generate preshared key: %w", err)
	}
	return key.String(), nil
}

// PublicKeyFromPrivate derives the public key from a base64-encoded private key.
func PublicKeyFromPrivate(privKeyBase64 string) (string, error) {
	key, err := wgtypes.ParseKey(privKeyBase64)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	return key.PublicKey().String(), nil
}

// ParseKey validates and parses a base64-encoded WireGuard key.
func ParseKey(s string) error {
	_, err := wgtypes.ParseKey(s)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}
	return nil
}
