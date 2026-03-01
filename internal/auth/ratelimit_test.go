package auth

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_AllowWithinLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	rl.RecordFailure("1.2.3.4")
	rl.RecordFailure("1.2.3.4")
	if !rl.Allow("1.2.3.4") {
		t.Error("Allow should be true when under the limit")
	}
}

func TestRateLimiter_DenyAtLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	rl.RecordFailure("1.2.3.4")
	rl.RecordFailure("1.2.3.4")
	rl.RecordFailure("1.2.3.4")
	if rl.Allow("1.2.3.4") {
		t.Error("Allow should be false when at the limit")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)
	rl.RecordFailure("10.0.0.1")
	if rl.Allow("10.0.0.1") {
		t.Error("Allow should be false right after limit reached")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.Allow("10.0.0.1") {
		t.Error("Allow should be true after window expires")
	}
}

func TestRateLimiter_RetryAfterPositive(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	rl.RecordFailure("5.5.5.5")
	rl.RecordFailure("5.5.5.5")
	after := rl.RetryAfter("5.5.5.5")
	if after <= 0 {
		t.Errorf("RetryAfter = %d, want > 0 when rate-limited", after)
	}
}

func TestRateLimiter_RetryAfterZeroWhenNotLimited(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	if after := rl.RetryAfter("9.9.9.9"); after != 0 {
		t.Errorf("RetryAfter = %d, want 0 when not rate-limited", after)
	}
}

func TestRateLimiter_ConcurrentSafety(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.RecordFailure("concurrent-ip")
			rl.Allow("concurrent-ip")
		}()
	}
	wg.Wait()
}
