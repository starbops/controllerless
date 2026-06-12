package dispatch

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsFirstCall(t *testing.T) {
	rl := newRateLimiter()
	if !rl.allow("default/myobj", "my-skill") {
		t.Fatal("first call should be allowed")
	}
}

func TestRateLimiter_BlocksSecondWithin5s(t *testing.T) {
	rl := newRateLimiter()
	rl.allow("default/myobj", "my-skill")
	if rl.allow("default/myobj", "my-skill") {
		t.Fatal("second call within 5s should be blocked")
	}
}

func TestRateLimiter_AllowsAfter5s(t *testing.T) {
	rl := newRateLimiter()
	key := rateLimitKey("default/myobj", "my-skill")

	rl.allow("default/myobj", "my-skill")
	// Backdate the last-fire time to simulate 5s elapsed.
	rl.mu.Lock()
	rl.lastFire[key] = time.Now().Add(-5 * time.Second)
	rl.mu.Unlock()

	if !rl.allow("default/myobj", "my-skill") {
		t.Fatal("call after 5s should be allowed")
	}
}

func TestRateLimiter_IndependentKeys(t *testing.T) {
	rl := newRateLimiter()
	rl.allow("default/obj1", "skill-a")
	// Different object key — should still be allowed.
	if !rl.allow("default/obj2", "skill-a") {
		t.Fatal("different resource key should be independent")
	}
	// Different skill — should still be allowed.
	if !rl.allow("default/obj1", "skill-b") {
		t.Fatal("different skill name should be independent")
	}
}
