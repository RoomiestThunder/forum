package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeClock lets the limiter tests move time explicitly instead of sleeping.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(limit int, window time.Duration) (*RateLimiter, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(limit, window)
	rl.now = clock.now
	return rl, clock
}

func TestRateLimiterBurstThenDeny(t *testing.T) {
	rl, _ := newTestLimiter(5, time.Minute)

	for i := 1; i <= 5; i++ {
		if ok, _ := rl.Allow("ip"); !ok {
			t.Fatalf("request %d within the burst should be allowed", i)
		}
	}
	ok, retry := rl.Allow("ip")
	if ok {
		t.Fatal("request past the burst should be denied")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retry-after should be within (0, window], got %v", retry)
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	rl, _ := newTestLimiter(1, time.Minute)

	if ok, _ := rl.Allow("a"); !ok {
		t.Fatal("first request for a should be allowed")
	}
	if ok, _ := rl.Allow("b"); !ok {
		t.Fatal("first request for b should be allowed")
	}
	if ok, _ := rl.Allow("a"); ok {
		t.Fatal("second request for a should be denied")
	}
}

func TestRateLimiterRefillsGradually(t *testing.T) {
	// 60/minute is one token per second.
	rl, clock := newTestLimiter(60, time.Minute)

	for i := 0; i < 60; i++ {
		rl.Allow("ip")
	}
	if ok, _ := rl.Allow("ip"); ok {
		t.Fatal("bucket should be empty after the burst")
	}

	clock.advance(time.Second)
	if ok, _ := rl.Allow("ip"); !ok {
		t.Fatal("one token should have refilled after a second")
	}
	if ok, _ := rl.Allow("ip"); ok {
		t.Fatal("only one token should have refilled, not the whole bucket")
	}
}

func TestRateLimiterNeverExceedsBurst(t *testing.T) {
	rl, clock := newTestLimiter(3, time.Minute)

	rl.Allow("ip")
	clock.advance(24 * time.Hour) // far longer than needed to refill

	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := rl.Allow("ip"); ok {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("idle time must not bank more than the burst: allowed %d, want 3", allowed)
	}
}

// A steady stream at the limit must not get a free burst every window - the
// previous implementation reset the whole bucket each minute, which let a
// client double its allowance across a window boundary.
func TestRateLimiterNoWindowResetWindfall(t *testing.T) {
	rl, clock := newTestLimiter(10, time.Minute)

	for i := 0; i < 10; i++ {
		rl.Allow("ip")
	}
	clock.advance(59 * time.Second) // just short of a full window

	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := rl.Allow("ip"); ok {
			allowed++
		}
	}
	// 59s at 10/min refills ~9.8 tokens, never the full 10.
	if allowed >= 10 {
		t.Fatalf("bucket refilled completely before the window elapsed: allowed %d", allowed)
	}
}

func TestRateLimiterEvictsIdleKeys(t *testing.T) {
	rl, clock := newTestLimiter(5, time.Minute)

	for _, key := range []string{"a", "b", "c"} {
		rl.Allow(key)
	}
	if got := rl.size(); got != 3 {
		t.Fatalf("want 3 tracked keys, got %d", got)
	}

	clock.advance(2 * time.Minute)
	rl.Allow("d") // any call past the sweep interval triggers eviction

	if got := rl.size(); got != 1 {
		t.Fatalf("idle keys should have been evicted, leaving only d: got %d keys", got)
	}
}

func TestRateLimiterEvictionKeepsActiveKeys(t *testing.T) {
	rl, clock := newTestLimiter(2, time.Minute)

	rl.Allow("busy")
	rl.Allow("busy") // drains the bucket

	clock.advance(30 * time.Second)
	rl.Allow("other")
	clock.advance(31 * time.Second) // sweep is due, but "busy" last seen 61s ago

	if ok, _ := rl.Allow("busy"); !ok {
		t.Fatal("an evicted-then-recreated bucket starts full, so this must be allowed")
	}
}

func TestRateLimiterZeroConfigDeniesEverything(t *testing.T) {
	for name, rl := range map[string]*RateLimiter{
		"zero limit":  NewRateLimiter(0, time.Minute),
		"zero window": NewRateLimiter(10, 0),
	} {
		if ok, _ := rl.Allow("ip"); ok {
			t.Errorf("%s: a misconfigured limiter must fail closed", name)
		}
	}
}

func TestRateLimiterConcurrentUse(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				rl.Allow("shared")
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	// Exactly the burst is admitted no matter how requests interleave (the
	// race detector additionally checks the locking).
	if ok, _ := rl.Allow("shared"); ok {
		t.Fatal("bucket should be exhausted after 1600 attempts against a burst of 100")
	}
}

func TestSecurityHeaders(t *testing.T) {
	called := false
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatal("wrapped handler was not invoked")
	}
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy header missing")
	}
}
