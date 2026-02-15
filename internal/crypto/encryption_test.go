package crypto

import (
	"testing"
)

func TestDeriveKey(t *testing.T) {
	secret := []byte("test-jwt-secret-value")

	key1, err := DeriveKey(secret)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	// Same secret produces same key.
	key2, err := DeriveKey(secret)
	if err != nil {
		t.Fatalf("DeriveKey second call: %v", err)
	}
	if key1 != key2 {
		t.Fatal("DeriveKey: same secret should produce same key")
	}

	// Different secret produces different key.
	key3, err := DeriveKey([]byte("different-secret"))
	if err != nil {
		t.Fatalf("DeriveKey different secret: %v", err)
	}
	if key1 == key3 {
		t.Fatal("DeriveKey: different secrets should produce different keys")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key, err := DeriveKey([]byte("test-secret"))
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"wireguard_key", "YNqHbfBQKaGvlC5A5QpSDA/KnNaNW+djkLAsMnBGcW8="},
		{"short_string", "hello"},
		{"empty_string", ""},
		{"preshared_key", "0000000000000000000000000000000000000000000="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			if tt.plaintext == "" {
				if encrypted != "" {
					t.Fatal("empty plaintext should produce empty ciphertext")
				}
				return
			}

			if encrypted == tt.plaintext {
				t.Fatal("encrypted should differ from plaintext")
			}

			decrypted, err := Decrypt(encrypted, key)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncrypt_DifferentCiphertexts(t *testing.T) {
	key, _ := DeriveKey([]byte("test-secret"))
	plaintext := "same-plaintext"

	enc1, _ := Encrypt(plaintext, key)
	enc2, _ := Encrypt(plaintext, key)

	if enc1 == enc2 {
		t.Fatal("encrypting same plaintext should produce different ciphertexts (random nonce)")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, _ := DeriveKey([]byte("secret-1"))
	key2, _ := DeriveKey([]byte("secret-2"))

	encrypted, _ := Encrypt("sensitive-data", key1)

	_, err := Decrypt(encrypted, key2)
	if err == nil {
		t.Fatal("decrypting with wrong key should fail")
	}
}

func TestIsEncrypted(t *testing.T) {
	key, _ := DeriveKey([]byte("test-secret"))

	// A WireGuard key (44-char base64 = 32 bytes decoded) should not be detected as encrypted.
	wgKey := "YNqHbfBQKaGvlC5A5QpSDA/KnNaNW+djkLAsMnBGcW8="
	if IsEncrypted(wgKey) {
		t.Fatal("WireGuard key should not be detected as encrypted")
	}

	// An encrypted value should be detected.
	encrypted, _ := Encrypt(wgKey, key)
	if !IsEncrypted(encrypted) {
		t.Fatal("encrypted value should be detected as encrypted")
	}

	// Empty string is not encrypted.
	if IsEncrypted("") {
		t.Fatal("empty string should not be detected as encrypted")
	}

	// Invalid base64 is not encrypted.
	if IsEncrypted("not-base64!!!") {
		t.Fatal("invalid base64 should not be detected as encrypted")
	}
}
