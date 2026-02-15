package auth

import (
	"sync"
	"time"
)

// LoginRateLimiter tracks login attempts per IP using a sliding window.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
	done     chan struct{}
}

// NewLoginRateLimiter creates a rate limiter that allows limit attempts
// per window per IP. Starts a background cleanup goroutine.
func NewLoginRateLimiter(limit int, window time.Duration) (*LoginRateLimiter, error) {
	l := &LoginRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		done:     make(chan struct{}),
	}
	go l.cleanup()
	return l, nil
}

// Allow checks whether the given IP is allowed to attempt a login.
// Returns true if under the rate limit, false if exceeded.
func (l *LoginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Remove expired entries for this IP.
	timestamps := l.attempts[ip]
	valid := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}

	if len(valid) >= l.limit {
		l.attempts[ip] = valid
		return false
	}

	l.attempts[ip] = append(valid, now)
	return true
}

// Stop terminates the background cleanup goroutine.
func (l *LoginRateLimiter) Stop() {
	close(l.done)
}

func (l *LoginRateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-l.window)
			for ip, timestamps := range l.attempts {
				valid := timestamps[:0]
				for _, ts := range timestamps {
					if ts.After(cutoff) {
						valid = append(valid, ts)
					}
				}
				if len(valid) == 0 {
					delete(l.attempts, ip)
				} else {
					l.attempts[ip] = valid
				}
			}
			l.mu.Unlock()
		}
	}
}
