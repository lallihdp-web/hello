package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLeakyBucket(t *testing.T) {
	lb := NewLeakyBucket(nil)
	if lb == nil {
		t.Fatal("expected non-nil leaky bucket")
	}
}

func TestLeakyBucket_Allow(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 5,
		LeakRate:   0, // Zero leak rate for strict testing
		QueueSize:  0,
	})

	// Should allow up to bucket size
	for i := 0; i < 5; i++ {
		if !lb.Allow("test-key") {
			t.Errorf("expected request %d to be allowed", i)
		}
	}

	// Should reject when bucket is full (no leaking)
	if lb.Allow("test-key") {
		t.Error("expected request to be rejected when bucket is full")
	}
}

func TestLeakyBucket_LeakRate(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 2,
		LeakRate:   100, // 100 per second = leaks fast
		QueueSize:  0,
	})

	// Fill the bucket quickly
	lb.Allow("test-key")
	lb.Allow("test-key")

	// Immediately try again - may or may not be allowed depending on timing
	// Wait a bit then try again
	time.Sleep(50 * time.Millisecond)

	// Should have leaked by now and allow more
	if !lb.Allow("test-key") {
		t.Error("expected bucket to have leaked and allow request")
	}
}

func TestLeakyBucket_DifferentKeys(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 2,
		LeakRate:   0, // Zero leak rate for strict testing
		QueueSize:  0,
	})

	// Fill bucket for key1
	lb.Allow("key1")
	lb.Allow("key1")

	// key1 should be full
	if lb.Allow("key1") {
		t.Error("expected key1 to be full")
	}

	// key2 should still be allowed (different bucket)
	if !lb.Allow("key2") {
		t.Error("expected key2 to be allowed")
	}
}

func TestLeakyBucket_Disabled(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    false,
		BucketSize: 1,
		LeakRate:   1,
	})

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !lb.Allow("test-key") {
			t.Error("expected all requests to be allowed when disabled")
		}
	}
}

func TestLeakyBucket_AllowByRole(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 2,
		LeakRate:   0, // Zero leak rate for strict testing
		RoleConfigs: map[string]*LeakyBucketRoleConfig{
			"admin": {
				BucketSize: 100,
				LeakRate:   0,
				QueueSize:  50,
			},
			"user": {
				BucketSize: 3,
				LeakRate:   0,
				QueueSize:  2,
			},
		},
	})

	// Admin should have higher limits - fill up to 100
	for i := 0; i < 100; i++ {
		if !lb.AllowByRole("admin", "admin-1") {
			t.Errorf("expected admin request %d to be allowed", i)
		}
	}

	// Admin bucket should be full now
	if lb.AllowByRole("admin", "admin-1") {
		t.Error("expected admin to be rate limited after 100 requests")
	}

	// User should have lower limits
	for i := 0; i < 3; i++ {
		if !lb.AllowByRole("user", "user-1") {
			t.Errorf("expected user request %d to be allowed", i)
		}
	}

	// User should be limited after 3
	if lb.AllowByRole("user", "user-1") {
		t.Error("expected user to be rate limited")
	}
}

func TestLeakyBucket_WithQueue(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:      true,
		BucketSize:   2,
		LeakRate:     100, // Fast leak for testing
		QueueSize:    5,
		QueueTimeout: 5 * time.Second,
	})

	ctx := context.Background()

	// Fill the bucket
	lb.Allow("test-key")
	lb.Allow("test-key")

	// This should queue and eventually succeed due to fast leak rate
	var wg sync.WaitGroup
	var allowed int32

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := lb.AllowWithQueue(ctx, "test-key")
			if err == nil && result {
				atomic.AddInt32(&allowed, 1)
			}
		}()
	}

	// Wait for queued requests to be processed
	wg.Wait()

	// At least some should have been allowed after leaking
	if allowed == 0 {
		t.Error("expected some queued requests to be allowed")
	}
}

func TestLeakyBucket_QueueFull(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:      true,
		BucketSize:   1,
		LeakRate:     0.001, // Very slow leak
		QueueSize:    1,
		QueueTimeout: 100 * time.Millisecond,
	})

	ctx := context.Background()

	// Fill bucket
	lb.Allow("test-key")

	// Fill queue with a goroutine
	go func() {
		lb.AllowWithQueue(ctx, "test-key")
	}()
	time.Sleep(20 * time.Millisecond) // Give time for queue to fill

	// This should fail - queue is full
	result, _ := lb.AllowWithQueue(ctx, "test-key")
	if result {
		t.Error("expected request to be rejected when queue is full")
	}
}

func TestLeakyBucket_GetStats(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 10,
		LeakRate:   0, // Zero leak rate for strict testing
		QueueSize:  0, // No queue to ensure requests are dropped
	})

	// Generate some traffic
	for i := 0; i < 15; i++ {
		lb.Allow("test-key")
	}

	stats := lb.GetStats("test-key")
	if stats == nil {
		t.Fatal("expected stats to be returned")
	}

	if stats.TotalRequests != 15 {
		t.Errorf("expected 15 total requests, got %d", stats.TotalRequests)
	}
	if stats.DroppedRequests != 5 {
		t.Errorf("expected 5 dropped requests, got %d", stats.DroppedRequests)
	}
	if stats.BucketSize != 10 {
		t.Errorf("expected bucket size 10, got %d", stats.BucketSize)
	}
}

func TestLeakyBucket_Middleware(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 2,
		LeakRate:   0, // Zero leak rate for strict testing
		QueueSize:  0,
		RoleConfigs: map[string]*LeakyBucketRoleConfig{
			"anonymous": {
				BucketSize: 2,
				LeakRate:   0,
				QueueSize:  0,
			},
		},
	})

	handler := lb.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected request %d to succeed, got %d", i, w.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}

	// Check header
	if w.Header().Get("X-RateLimit-Type") != "leaky-bucket" {
		t.Error("expected X-RateLimit-Type header")
	}
}

func TestLeakyBucket_MiddlewareWithQueue(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:      true,
		BucketSize:   1,
		LeakRate:     100, // Fast leak
		QueueSize:    5,
		QueueTimeout: time.Second,
		RoleConfigs: map[string]*LeakyBucketRoleConfig{
			"anonymous": {
				BucketSize:   1,
				LeakRate:     100,
				QueueSize:    5,
				QueueTimeout: time.Second,
			},
		},
	})

	var processedCount int32
	handler := lb.MiddlewareWithQueue(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&processedCount, 1)
		w.WriteHeader(http.StatusOK)
	}))

	// Send multiple concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	// All requests should have been processed (queued and leaked)
	if processedCount == 0 {
		t.Error("expected some requests to be processed")
	}
}

func TestLeakyBucket_StatsNonExistent(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 10,
		LeakRate:   1,
	})

	// Get stats for non-existent key
	stats := lb.GetStats("nonexistent")
	if stats != nil {
		t.Error("expected nil stats for nonexistent key")
	}
}

func TestLeakyBucket_AllowByEndpoint(t *testing.T) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 10,
		LeakRate:   0, // Zero leak rate for strict testing
		EndpointConfigs: map[string]*LeakyBucketRoleConfig{
			"/api/heavy": {
				BucketSize: 2,
				LeakRate:   0,
				QueueSize:  0,
			},
		},
	})

	// Heavy endpoint should have low limits
	lb.AllowByEndpoint("/api/heavy", "user1")
	lb.AllowByEndpoint("/api/heavy", "user1")

	if lb.AllowByEndpoint("/api/heavy", "user1") {
		t.Error("expected heavy endpoint to be rate limited")
	}

	// Other endpoints use default limits (10)
	for i := 0; i < 10; i++ {
		if !lb.AllowByEndpoint("/api/light", "user1") {
			t.Errorf("expected light endpoint request %d to be allowed", i)
		}
	}
}

// Benchmarks

func BenchmarkLeakyBucket_Allow(b *testing.B) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 100000,
		LeakRate:   100000,
		QueueSize:  0,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.Allow("test-key")
	}
}

func BenchmarkLeakyBucket_AllowDifferentKeys(b *testing.B) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 100000,
		LeakRate:   100000,
		QueueSize:  0,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.Allow(string(rune(i % 1000)))
	}
}

func BenchmarkLeakyBucket_Concurrent(b *testing.B) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 100000,
		LeakRate:   100000,
		QueueSize:  0,
	})

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			lb.Allow(string(rune(i % 100)))
			i++
		}
	})
}

func BenchmarkLeakyBucket_Middleware(b *testing.B) {
	lb := NewLeakyBucket(&LeakyBucketConfig{
		Enabled:    true,
		BucketSize: 100000,
		LeakRate:   100000,
		QueueSize:  0,
		RoleConfigs: map[string]*LeakyBucketRoleConfig{
			"anonymous": {BucketSize: 100000, LeakRate: 100000},
		},
	})

	handler := lb.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
