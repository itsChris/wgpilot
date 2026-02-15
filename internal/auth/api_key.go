package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	apiKeyLength = 32 // 32 bytes = 64 hex chars
	apiKeyPrefix = "wgp_"
)

// GenerateAPIKey generates a random API key and returns the key, its SHA-256 hash,
// and a display prefix.
func GenerateAPIKey() (key, hash, prefix string, err error) {
	b := make([]byte, apiKeyLength)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}

	key = apiKeyPrefix + hex.EncodeToString(b)
	hash = HashAPIKey(key)
	prefix = key[:len(apiKeyPrefix)+8] + "..."

	return key, hash, prefix, nil
}

// HashAPIKey returns the SHA-256 hash of an API key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
