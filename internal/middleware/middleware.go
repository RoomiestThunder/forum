package middleware

import (
	"net/http"
	"sync"
	"time"
)

// SecurityHeaders adds standard hardening headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https:; "+
			"script-src 'self' https://unpkg.com; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
			"font-src 'self' https://fonts.gstatic.com; connect-src 'self' ws: wss:")
		next.ServeHTTP(w, r)
	})
}

// RateLimiter implements a simple per-key token bucket, used as a fallback
// rate limiter when Redis is not configured.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	limit   int // tokens added per second
	burst   int // maximum bucket size
	reset   time.Duration
}

type tokenBucket struct {
	tokens    float64
	lastSeen  time.Time
	lastReset time.Time
}

// NewRateLimiter creates a rate limiter allowing requestsPerSecond tokens/sec
// with the given burst capacity per key.
func NewRateLimiter(requestsPerSecond, burst int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		limit:   requestsPerSecond,
		burst:   burst,
		reset:   time.Minute,
	}
}

// Allow reports whether a request from the given key (typically an IP) is
// allowed under the current bucket state.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	bucket, exists := rl.buckets[key]

	if !exists || now.Sub(bucket.lastReset) > rl.reset {
		rl.buckets[key] = &tokenBucket{
			tokens:    float64(rl.burst),
			lastSeen:  now,
			lastReset: now,
		}
		bucket = rl.buckets[key]
	}

	elapsed := now.Sub(bucket.lastSeen).Seconds()
	bucket.tokens = min(float64(rl.burst), bucket.tokens+elapsed*float64(rl.limit))
	bucket.lastSeen = now

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
