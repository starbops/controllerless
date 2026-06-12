package dispatch

import (
	"fmt"
	"sync"
	"time"
)

const rateLimitWindow = 5 * time.Second

// rateLimiter enforces S3: at most one reconcile per (resource-key, skill-name) per 5s.
type rateLimiter struct {
	mu       sync.Mutex
	lastFire map[string]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{lastFire: make(map[string]time.Time)}
}

func rateLimitKey(resourceKey, skillName string) string {
	return fmt.Sprintf("%s\x00%s", resourceKey, skillName)
}

// allow returns true and records the fire time if the window has elapsed.
// Returns false if called again within rateLimitWindow.
func (rl *rateLimiter) allow(resourceKey, skillName string) bool {
	key := rateLimitKey(resourceKey, skillName)
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if last, ok := rl.lastFire[key]; ok && now.Sub(last) < rateLimitWindow {
		return false
	}
	rl.lastFire[key] = now
	return true
}
