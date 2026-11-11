package middleware

import (
	"sync"
	"time"
)

// RateLimiter is a per-key token bucket: every key may burst up to `limit`
// requests and then sustains limit/window requests per second.
//
// It backs the API rate limits when Redis is not configured (or is briefly
// unreachable), so it has to stay bounded under hostile traffic: buckets that
// have been idle long enough to be full again are indistinguishable from a
// fresh bucket, so they are evicted rather than kept forever.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   int
	window  time.Duration
	rate    float64 // tokens added per second
	now     func() time.Time

	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter allows `limit` requests per `window` for each key. A
// non-positive limit or window yields a limiter that denies everything, which
// is the safe failure mode for a misconfiguration.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
	if limit > 0 && window > 0 {
		rl.rate = float64(limit) / window.Seconds()
	}
	return rl
}

// Allow reports whether a request for key may proceed. When it may not, the
// returned duration is how long until a token is available again, suitable
// for a Retry-After header.
func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	if rl.rate <= 0 {
		return false, rl.window
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	rl.sweep(now)

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.limit), last: now}
		rl.buckets[key] = b
	}

	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(float64(rl.limit), b.tokens+elapsed*rl.rate)
	b.last = now

	if b.tokens < 1 {
		wait := time.Duration((1 - b.tokens) / rl.rate * float64(time.Second))
		return false, wait
	}
	b.tokens--
	return true, 0
}

// sweep drops buckets that have been idle for a full window (and are therefore
// full again). It runs at most once per window, so the O(n) scan is amortised
// across requests and needs no background goroutine to start or stop.
func (rl *RateLimiter) sweep(now time.Time) {
	if now.Sub(rl.lastSweep) < rl.window {
		return
	}
	rl.lastSweep = now
	for key, b := range rl.buckets {
		if now.Sub(b.last) >= rl.window {
			delete(rl.buckets, key)
		}
	}
}

// size reports the number of tracked keys; used by tests to assert eviction.
func (rl *RateLimiter) size() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}
