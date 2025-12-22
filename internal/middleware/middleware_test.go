package middleware

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(10, 5)

	for i := 0; i < 5; i++ {
		if !limiter.Allow("test_ip") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if limiter.Allow("test_ip") {
		t.Error("Request 6 should be denied")
	}
}

func TestRateLimiterMultipleIPs(t *testing.T) {
	limiter := NewRateLimiter(1, 1)

	if !limiter.Allow("ip1") {
		t.Error("IP1 first request should be allowed")
	}

	if !limiter.Allow("ip2") {
		t.Error("IP2 first request should be allowed")
	}

	if limiter.Allow("ip1") {
		t.Error("IP1 second request should be denied")
	}

	if limiter.Allow("ip2") {
		t.Error("IP2 second request should be denied")
	}
}

func TestRateLimiterTokenRefill(t *testing.T) {
	limiter := NewRateLimiter(1000, 1)

	if !limiter.Allow("ip") {
		t.Error("First request should be allowed")
	}

	if limiter.Allow("ip") {
		t.Error("Second request should be denied (no tokens)")
	}

	time.Sleep(2 * time.Millisecond)

	if !limiter.Allow("ip") {
		t.Error("Request after token refill should be allowed")
	}
}

func TestNewRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(10, 5)

	if limiter.limit != 10 {
		t.Errorf("Expected limit 10, got %d", limiter.limit)
	}

	if limiter.burst != 5 {
		t.Errorf("Expected burst 5, got %d", limiter.burst)
	}

	if len(limiter.buckets) != 0 {
		t.Error("Expected empty buckets on creation")
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a        float64
		b        float64
		expected float64
	}{
		{1.0, 2.0, 1.0},
		{2.0, 1.0, 1.0},
		{1.5, 1.5, 1.5},
		{-1.0, 1.0, -1.0},
		{0.0, 0.0, 0.0},
	}

	for _, tt := range tests {
		if min(tt.a, tt.b) != tt.expected {
			t.Errorf("min(%f, %f): expected %f, got %f", tt.a, tt.b, tt.expected, min(tt.a, tt.b))
		}
	}
}
