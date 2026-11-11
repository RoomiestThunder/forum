package cache

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// newTestCache connects to a real Redis (the rate limiter is a Lua script, so
// it is only meaningfully tested against the real thing). The tests skip when
// none is reachable, so `go test ./...` still works on a bare machine; CI
// provides one through REDIS_URL.
func newTestCache(t *testing.T) *Cache {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379"
	}
	c, err := New(url)
	if err != nil {
		t.Skipf("redis not available at %s: %v", url, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// uniqueKey namespaces a key to the test and removes it afterwards, so tests
// never collide with each other or with data in a shared Redis.
func uniqueKey(t *testing.T, c *Cache, name string) string {
	t.Helper()
	key := fmt.Sprintf("test:%s:%s:%d", t.Name(), name, time.Now().UnixNano())
	t.Cleanup(func() { c.client.Del(context.Background(), key) })
	return key
}

func TestAllowEnforcesLimit(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := uniqueKey(t, c, "k")

	for i := 1; i <= 3; i++ {
		ok, _, err := c.Allow(ctx, key, 3, time.Minute)
		if err != nil || !ok {
			t.Fatalf("request %d within the limit: ok=%v err=%v", i, ok, err)
		}
	}

	ok, retry, err := c.Allow(ctx, key, 3, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("request over the limit should be denied")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retry-after should be in (0, window], got %v", retry)
	}
}

func TestAllowKeysAreIndependent(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	a, b := uniqueKey(t, c, "a"), uniqueKey(t, c, "b")

	if ok, _, _ := c.Allow(ctx, a, 1, time.Minute); !ok {
		t.Fatal("first request for a should be allowed")
	}
	if ok, _, _ := c.Allow(ctx, a, 1, time.Minute); ok {
		t.Fatal("second request for a should be denied")
	}
	if ok, _, _ := c.Allow(ctx, b, 1, time.Minute); !ok {
		t.Fatal("b must not be affected by a's usage")
	}
}

func TestAllowWindowExpires(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := uniqueKey(t, c, "k")
	window := 300 * time.Millisecond

	if ok, _, err := c.Allow(ctx, key, 1, window); err != nil || !ok {
		t.Fatalf("first request should be allowed: ok=%v err=%v", ok, err)
	}
	if ok, _, _ := c.Allow(ctx, key, 1, window); ok {
		t.Fatal("second request inside the window should be denied")
	}

	time.Sleep(window + 150*time.Millisecond)

	if ok, _, err := c.Allow(ctx, key, 1, window); err != nil || !ok {
		t.Fatalf("a new window should allow requests again: ok=%v err=%v", ok, err)
	}
}

// Hitting a limited key must not extend its window: otherwise a client that
// keeps retrying would keep pushing its own reset further out.
func TestAllowDeniedRequestsDoNotExtendWindow(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := uniqueKey(t, c, "k")

	if ok, _, err := c.Allow(ctx, key, 1, 2*time.Second); err != nil || !ok {
		t.Fatalf("first request should be allowed: ok=%v err=%v", ok, err)
	}
	_, first, _ := c.Allow(ctx, key, 1, 2*time.Second)
	time.Sleep(300 * time.Millisecond)
	_, second, _ := c.Allow(ctx, key, 1, 2*time.Second)

	if second >= first {
		t.Fatalf("remaining window should shrink as time passes, not reset: first=%v second=%v", first, second)
	}
}

// A counter that has lost its TTL (a crash between INCR and EXPIRE in the old
// two-call implementation) would otherwise block its key forever.
func TestAllowRepairsCounterWithoutTTL(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	key := uniqueKey(t, c, "k")

	if err := c.client.Set(ctx, key, 100, 0).Err(); err != nil { // no expiry
		t.Fatal(err)
	}

	ok, retry, err := c.Allow(ctx, key, 5, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("counter is already over the limit, request should be denied")
	}
	if retry <= 0 {
		t.Fatalf("a TTL must have been attached so the key can recover, retry-after=%v", retry)
	}
	if ttl := c.client.PTTL(ctx, key).Val(); ttl <= 0 {
		t.Fatalf("key still has no TTL: %v", ttl)
	}
}

func TestAllowReportsRedisErrors(t *testing.T) {
	c := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // a dead context makes the call fail without needing Redis to be down

	if _, _, err := c.Allow(ctx, "test:ignored", 1, time.Minute); err == nil {
		t.Fatal("an unreachable/failed Redis call must surface as an error so the caller can fall back")
	}
}
