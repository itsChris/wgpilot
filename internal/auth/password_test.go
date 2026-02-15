package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("testpassword123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}

	// Verify bcrypt cost is 12.
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost failed: %v", err)
	}
	if cost != DefaultBcryptCost {
		t.Errorf("expected cost %d, got %d", DefaultBcryptCost, cost)
	}
}

func TestVerifyPassword_Correct(t *testing.T) {
	password := "correctpassword"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if err := VerifyPassword(hash, password); err != nil {
		t.Errorf("VerifyPassword should succeed for correct password: %v", err)
	}
}

func TestVerifyPassword_Wrong(t *testing.T) {
	hash, err := HashPassword("correctpassword")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if err := VerifyPassword(hash, "wrongpassword"); err == nil {
		t.Error("VerifyPassword should fail for wrong password")
	}
}

func TestGenerateOTP_Length(t *testing.T) {
	otp, err := GenerateOTP(16)
	if err != nil {
		t.Fatalf("GenerateOTP failed: %v", err)
	}
	if len(otp) != 16 {
		t.Errorf("expected length 16, got %d", len(otp))
	}
}

func TestGenerateOTP_Unique(t *testing.T) {
	otp1, err := GenerateOTP(16)
	if err != nil {
		t.Fatalf("GenerateOTP failed: %v", err)
	}
	otp2, err := GenerateOTP(16)
	if err != nil {
		t.Fatalf("GenerateOTP failed: %v", err)
	}
	if otp1 == otp2 {
		t.Error("two generated OTPs should not be equal")
	}
}

func TestMinPasswordLength_Is10(t *testing.T) {
	if MinPasswordLength != 10 {
		t.Errorf("expected MinPasswordLength=10, got %d", MinPasswordLength)
	}
}

func TestMinPasswordLength_Enforcement(t *testing.T) {
	tests := []struct {
		name     string
		password string
		tooShort bool
	}{
		{"empty", "", true},
		{"1 char", "a", true},
		{"9 chars", "123456789", true},
		{"10 chars", "1234567890", false},
		{"11 chars", "12345678901", false},
		{"72 chars", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isShort := len(tt.password) < MinPasswordLength
			if isShort != tt.tooShort {
				t.Errorf("password %q: expected tooShort=%v, got %v", tt.password, tt.tooShort, isShort)
			}
		})
	}
}

func TestGenerateOTP_Charset(t *testing.T) {
	otp, err := GenerateOTP(100)
	if err != nil {
		t.Fatalf("GenerateOTP failed: %v", err)
	}
	for _, c := range otp {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			t.Errorf("unexpected character in OTP: %c", c)
		}
	}
}
