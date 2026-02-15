package auth

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testSecret() []byte {
	return []byte("test-secret-key-that-is-32-bytes!")
}

func TestNewJWTService_ShortSecret(t *testing.T) {
	_, err := NewJWTService([]byte("short"), time.Hour, testLogger())
	if err == nil {
		t.Error("should reject secret shorter than 32 bytes")
	}
}

func TestJWTService_Generate(t *testing.T) {
	svc, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	token, err := svc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}
}

func TestJWTService_Validate_Valid(t *testing.T) {
	svc, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	token, err := svc.Generate(42, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := svc.Validate(token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if claims.Subject != "42" {
		t.Errorf("expected sub=42, got %q", claims.Subject)
	}
	if claims.Username != "admin" {
		t.Errorf("expected username=admin, got %q", claims.Username)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role=admin, got %q", claims.Role)
	}
}

func TestJWTService_Validate_Expired(t *testing.T) {
	svc, err := NewJWTService(testSecret(), time.Nanosecond, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	token, err := svc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Wait for token to expire.
	time.Sleep(2 * time.Millisecond)

	_, err = svc.Validate(token)
	if err == nil {
		t.Error("Validate should fail for expired token")
	}
}

func TestJWTService_Validate_WrongSecret(t *testing.T) {
	svc1, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	token, err := svc1.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	svc2, err := NewJWTService([]byte("different-secret-also-32-bytes!!"), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	_, err = svc2.Validate(token)
	if err == nil {
		t.Error("Validate should fail with different secret")
	}
}

func TestJWTService_Validate_Malformed(t *testing.T) {
	svc, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	_, err = svc.Validate("not.a.valid.token")
	if err == nil {
		t.Error("Validate should fail for malformed token")
	}
}

func TestJWTService_Validate_NoneAlgorithm(t *testing.T) {
	svc, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	// Create a token with "none" algorithm (alg=none attack).
	token := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username: "admin",
		Role:     "admin",
	})
	signed, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err = svc.Validate(signed)
	if err == nil {
		t.Error("Validate should reject none algorithm")
	}
}

func TestJWTService_Validate_TamperedSignature(t *testing.T) {
	svc, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	token, err := svc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Tamper with the signature by flipping a character.
	tampered := []byte(token)
	if tampered[len(tampered)-1] == 'A' {
		tampered[len(tampered)-1] = 'B'
	} else {
		tampered[len(tampered)-1] = 'A'
	}

	_, err = svc.Validate(string(tampered))
	if err == nil {
		t.Error("Validate should reject token with tampered signature")
	}
}

func TestJWTService_Validate_TamperedPayload(t *testing.T) {
	svc, err := NewJWTService(testSecret(), 24*time.Hour, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	token, err := svc.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Split JWT into header.payload.signature and tamper with the payload.
	parts := splitJWT(token)
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	// Modify the payload (flip a byte).
	payload := []byte(parts[1])
	if len(payload) > 5 {
		payload[5] ^= 0xFF
	}
	tampered := parts[0] + "." + string(payload) + "." + parts[2]

	_, err = svc.Validate(tampered)
	if err == nil {
		t.Error("Validate should reject token with tampered payload")
	}
}

func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}

func TestJWTService_TTL(t *testing.T) {
	ttl := 12 * time.Hour
	svc, err := NewJWTService(testSecret(), ttl, testLogger())
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	if svc.TTL() != ttl {
		t.Errorf("expected TTL %v, got %v", ttl, svc.TTL())
	}
}

func TestGenerateSecret(t *testing.T) {
	secret, err := GenerateSecret(32)
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(secret) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(secret))
	}

	// Check uniqueness.
	secret2, _ := GenerateSecret(32)
	if string(secret) == string(secret2) {
		t.Error("two secrets should not be equal")
	}
}
