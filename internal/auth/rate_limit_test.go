package auth

import (
	"testing"
	"time"
)

func TestAllow_UnderLimit(t *testing.T) {
	rl, err := NewLoginRateLimiter(5, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	defer rl.Stop()

	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("attempt %d should be allowed", i+1)
		}
	}
}

func TestAllow_AtLimit(t *testing.T) {
	rl, err := NewLoginRateLimiter(5, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	defer rl.Stop()

	for i := 0; i < 5; i++ {
		rl.Allow("192.168.1.1")
	}

	if rl.Allow("192.168.1.1") {
		t.Error("6th attempt should be denied")
	}
}

func TestAllow_DifferentIPs(t *testing.T) {
	rl, err := NewLoginRateLimiter(2, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	defer rl.Stop()

	// Exhaust limit for IP1.
	rl.Allow("ip1")
	rl.Allow("ip1")
	if rl.Allow("ip1") {
		t.Error("ip1 should be denied")
	}

	// IP2 should still be allowed.
	if !rl.Allow("ip2") {
		t.Error("ip2 should be allowed")
	}
}

func TestAllow_WindowReset(t *testing.T) {
	rl, err := NewLoginRateLimiter(2, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	defer rl.Stop()

	rl.Allow("192.168.1.1")
	rl.Allow("192.168.1.1")
	if rl.Allow("192.168.1.1") {
		t.Error("should be denied at limit")
	}

	// Wait for window to expire.
	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("192.168.1.1") {
		t.Error("should be allowed after window reset")
	}
}
