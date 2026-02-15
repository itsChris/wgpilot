package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost is the bcrypt work factor for password hashing.
	DefaultBcryptCost = 12

	// MinPasswordLength is the minimum acceptable password length.
	MinPasswordLength = 10
)

// HashPassword hashes a password using bcrypt with cost 12.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword checks a plaintext password against a bcrypt hash.
// Returns nil on success, error on mismatch or failure.
func VerifyPassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	return nil
}

// GenerateOTP creates a random alphanumeric one-time password of the given length.
func GenerateOTP(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("generate OTP: %w", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}
