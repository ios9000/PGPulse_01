package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter tracks failed login attempts per IP using a sliding window.
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time
	maxAttempts int
	window      time.Duration
}

// NewRateLimiter creates a RateLimiter that allows maxAttempts per window.
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
	}
}

// Allow reports whether ip is within the attempt limit.
// It does NOT record an attempt — call RecordFailure on failed logins.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.pruneExpired(ip)
	return len(rl.attempts[ip]) < rl.maxAttempts
}

// RecordFailure records a failed login attempt for ip.
func (rl *RateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

// RetryAfter returns the number of seconds until the window resets for ip.
// Returns 0 if the ip is not currently rate-limited.
func (rl *RateLimiter) RetryAfter(ip string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.pruneExpired(ip)
	entries := rl.attempts[ip]
	if len(entries) < rl.maxAttempts {
		return 0
	}
	remaining := rl.window - time.Since(entries[0])
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Seconds()) + 1
}

// pruneExpired removes attempts outside the sliding window for ip.
// Must be called with rl.mu held.
func (rl *RateLimiter) pruneExpired(ip string) {
	cutoff := time.Now().Add(-rl.window)
	entries := rl.attempts[ip]
	i := 0
	for i < len(entries) && entries[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		rl.attempts[ip] = entries[i:]
	}
	if len(rl.attempts[ip]) == 0 {
		delete(rl.attempts, ip)
	}
}

// ClientIP extracts the real client IP from the request.
// Checks X-Forwarded-For then X-Real-IP, then falls back to RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.Index(xff, ","); comma > 0 {
			xff = xff[:comma]
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
