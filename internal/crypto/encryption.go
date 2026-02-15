package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// nonceSize is the standard AES-GCM nonce length (12 bytes).
	nonceSize = 12
	// minEncryptedLen is the minimum base64-decoded length: nonce + at least 1 byte ciphertext + GCM tag.
	minEncryptedLen = nonceSize + 1 + 16
	// hkdfSalt is a fixed salt for key derivation.
	hkdfSalt = "wgpilot-key-encryption"
)

// DeriveKey derives a 32-byte encryption key from a master secret using HKDF-SHA256.
func DeriveKey(masterSecret []byte) ([32]byte, error) {
	var key [32]byte
	r := hkdf.New(sha256.New, masterSecret, []byte(hkdfSalt), nil)
	if _, err := io.ReadFull(r, key[:]); err != nil {
		return key, fmt.Errorf("derive encryption key: %w", err)
	}
	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns base64(nonce || ciphertext).
func Encrypt(plaintext string, key [32]byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("encrypt: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt: generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decodes base64(nonce || ciphertext) and decrypts using AES-256-GCM.
func Decrypt(encoded string, key [32]byte) (string, error) {
	if encoded == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decrypt: base64 decode: %w", err)
	}

	if len(data) < minEncryptedLen {
		return "", fmt.Errorf("decrypt: ciphertext too short (%d bytes)", len(data))
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("decrypt: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decrypt: new gcm: %w", err)
	}

	nonce := data[:gcm.NonceSize()]
	ciphertext := data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: gcm open: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted detects whether a value looks like an encrypted string.
// WireGuard keys are 44-char base64 (32 bytes). Encrypted values are
// longer because they include a 12-byte nonce + 16-byte GCM tag.
func IsEncrypted(s string) bool {
	if s == "" {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return false
	}
	// WireGuard base64 key = 32 bytes decoded.
	// Encrypted value = 12 (nonce) + 32 (key) + 16 (tag) = 60 bytes minimum.
	// So anything that decodes to >= minEncryptedLen and is NOT exactly 32 bytes
	// is likely encrypted.
	return len(data) >= minEncryptedLen && len(data) != 32
}
