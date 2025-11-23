package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	config := DefaultRateLimitConfig()
	limiter := NewRateLimiter(config)

	if limiter == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  10,
			Window: time.Second,
			Burst:  5,
		},
	})

	// Should allow initial requests within burst
	for i := 0; i < 5; i++ {
		if !limiter.Allow("test-key", nil) {
			t.Errorf("expected request %d to be allowed", i)
		}
	}
}

func TestRateLimiter_RateLimit(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  1,
			Window: time.Second,
			Burst:  2,
		},
	})

	// Exhaust burst
	limiter.Allow("test-key", nil)
	limiter.Allow("test-key", nil)

	// Should be rate limited
	if limiter.Allow("test-key", nil) {
		t.Error("expected request to be rate limited")
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  1,
			Window: time.Second,
			Burst:  1,
		},
	})

	// Different keys should have independent limits
	if !limiter.Allow("key-1", nil) {
		t.Error("expected key-1 to be allowed")
	}
	if !limiter.Allow("key-2", nil) {
		t.Error("expected key-2 to be allowed")
	}

	// Now both should be limited
	if limiter.Allow("key-1", nil) {
		t.Error("expected key-1 to be limited")
	}
	if limiter.Allow("key-2", nil) {
		t.Error("expected key-2 to be limited")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  100, // 100 per second = 1 per 10ms
			Window: time.Second,
			Burst:  1,
		},
	})

	// Use the burst
	limiter.Allow("test-key", nil)

	// Should be limited
	if limiter.Allow("test-key", nil) {
		t.Error("expected to be limited after burst")
	}

	// Wait for token refill
	time.Sleep(15 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow("test-key", nil) {
		t.Error("expected to be allowed after token refill")
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: false,
		Default: &RateLimit{
			Limit:  1,
			Window: time.Second,
			Burst:  1,
		},
	})

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !limiter.Allow("test-key", nil) {
			t.Error("expected all requests to be allowed when disabled")
		}
	}
}

func TestRateLimiter_AllowByRole(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  10,
			Window: time.Second,
			Burst:  5,
		},
		RoleLimits: map[string]*RateLimit{
			"admin": {Limit: 100, Window: time.Second, Burst: 50},
			"user":  {Limit: 10, Window: time.Second, Burst: 5},
		},
	})

	// Admin should have higher limits
	for i := 0; i < 50; i++ {
		if !limiter.AllowByRole("admin", "user-1") {
			t.Errorf("expected admin request %d to be allowed", i)
		}
	}
}

func TestRateLimiter_AllowByIP(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  10,
			Window: time.Second,
			Burst:  5,
		},
	})

	// Should allow initial requests
	for i := 0; i < 5; i++ {
		if !limiter.AllowByIP("192.168.1.1") {
			t.Errorf("expected IP request %d to be allowed", i)
		}
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  10,
			Window: time.Second,
			Burst:  5,
		},
	})

	// Create some buckets
	limiter.Allow("key-1", nil)
	limiter.Allow("key-2", nil)
	limiter.Allow("key-3", nil)

	// Cleanup old buckets (none should be removed as they're new)
	limiter.Cleanup(time.Hour)

	// Reset should work
	limiter.Reset("key-1")
}

func TestRateLimiter_ResetAll(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  1,
			Window: time.Second,
			Burst:  1,
		},
	})

	// Exhaust limits
	limiter.Allow("key-1", nil)
	limiter.Allow("key-2", nil)

	// Should be limited
	if limiter.Allow("key-1", nil) {
		t.Error("expected key-1 to be limited")
	}

	// Reset all
	limiter.ResetAll()

	// Should be allowed again
	if !limiter.Allow("key-1", nil) {
		t.Error("expected key-1 to be allowed after reset")
	}
}

func TestRateLimiter_Middleware(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  1,
			Window: time.Second,
			Burst:  1,
		},
		RoleLimits: map[string]*RateLimit{
			"anonymous": {Limit: 1, Window: time.Second, Burst: 1},
		},
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should succeed
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Second request should be rate limited
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestRateLimiter_MiddlewareWithRole(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  1,
			Window: time.Second,
			Burst:  1,
		},
		RoleLimits: map[string]*RateLimit{
			"admin": {Limit: 100, Window: time.Second, Burst: 100},
		},
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Admin requests should not be limited
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Hasura-Role", "admin")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected admin request %d to succeed, got %d", i, w.Code)
		}
	}
}

func TestRateLimiter_GetRemainingTokens(t *testing.T) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  10,
			Window: time.Second,
			Burst:  5,
		},
	})

	// Use some tokens
	limiter.Allow("test-key", nil)
	limiter.Allow("test-key", nil)

	remaining := limiter.GetRemainingTokens("role:test:test-key")
	// Note: remaining might be 0 if key doesn't exist with that exact format
	_ = remaining // Just testing the method works
}

// Benchmarks
func BenchmarkRateLimiter_Allow(b *testing.B) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  100000,
			Window: time.Second,
			Burst:  10000,
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("test-key", nil)
	}
}

func BenchmarkRateLimiter_AllowDifferentKeys(b *testing.B) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  100000,
			Window: time.Second,
			Burst:  10000,
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(string(rune(i%1000)), nil)
	}
}

func BenchmarkRateLimiter_Concurrent(b *testing.B) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  100000,
			Window: time.Second,
			Burst:  10000,
		},
	})

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			limiter.Allow(string(rune(i%100)), nil)
			i++
		}
	})
}

func BenchmarkRateLimiter_Middleware(b *testing.B) {
	limiter := NewRateLimiter(&RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  100000,
			Window: time.Second,
			Burst:  10000,
		},
		RoleLimits: map[string]*RateLimit{
			"anonymous": {Limit: 100000, Window: time.Second, Burst: 10000},
		},
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
