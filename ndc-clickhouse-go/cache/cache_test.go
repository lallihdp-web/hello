package cache

import (
	"testing"
	"time"
)

func TestNewInMemoryCache(t *testing.T) {
	cache := NewInMemoryCache(100)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if cache.maxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", cache.maxSize)
	}
}

func TestInMemoryCache_SetAndGet(t *testing.T) {
	cache := NewInMemoryCache(100)

	cache.Set("key1", "value1", time.Minute)

	value, found := cache.Get("key1")
	if !found {
		t.Fatal("expected to find cached value")
	}
	if value != "value1" {
		t.Errorf("expected value1, got %v", value)
	}
}

func TestInMemoryCache_Expiration(t *testing.T) {
	cache := NewInMemoryCache(100)

	cache.Set("key1", "value1", 10*time.Millisecond)

	// Should be found immediately
	_, found := cache.Get("key1")
	if !found {
		t.Error("expected to find value before expiration")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	_, found = cache.Get("key1")
	if found {
		t.Error("expected value to be expired")
	}
}

func TestInMemoryCache_MaxSize(t *testing.T) {
	cache := NewInMemoryCache(3)

	// Add 4 entries (exceeds max)
	for i := 0; i < 4; i++ {
		cache.Set(string(rune('a'+i)), i, time.Minute)
	}

	// Should have evicted oldest entry
	stats := cache.Stats()
	if stats.Size > 3 {
		t.Errorf("expected max 3 entries, got %d", stats.Size)
	}
	if stats.Evictions < 1 {
		t.Errorf("expected at least 1 eviction, got %d", stats.Evictions)
	}
}

func TestInMemoryCache_Delete(t *testing.T) {
	cache := NewInMemoryCache(100)

	cache.Set("key1", "value1", time.Minute)
	cache.Delete("key1")

	_, found := cache.Get("key1")
	if found {
		t.Error("expected value to be deleted")
	}
}

func TestInMemoryCache_Clear(t *testing.T) {
	cache := NewInMemoryCache(100)

	for i := 0; i < 5; i++ {
		cache.Set(string(rune('a'+i)), i, time.Minute)
	}

	cache.Clear()

	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected empty cache, got %d entries", stats.Size)
	}
}

func TestInMemoryCache_Stats(t *testing.T) {
	cache := NewInMemoryCache(100)

	cache.Set("key1", "value1", time.Minute)

	// Hits
	cache.Get("key1")
	cache.Get("key1")

	// Miss
	cache.Get("nonexistent")

	stats := cache.Stats()
	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestQueryCache_GetAndSet(t *testing.T) {
	cache := NewQueryCache(&QueryCacheConfig{
		Enabled:    true,
		MaxSize:    100,
		DefaultTTL: time.Minute,
	})

	query := map[string]interface{}{"limit": 10}
	result := []string{"item1", "item2"}

	cache.Set("users", query, result)

	cached, found := cache.Get("users", query)
	if !found {
		t.Fatal("expected to find cached query result")
	}

	cachedSlice, ok := cached.([]string)
	if !ok {
		t.Fatal("expected result to be []string")
	}
	if len(cachedSlice) != 2 {
		t.Errorf("expected 2 items, got %d", len(cachedSlice))
	}
}

func TestQueryCache_Disabled(t *testing.T) {
	cache := NewQueryCache(&QueryCacheConfig{
		Enabled:    false,
		MaxSize:    100,
		DefaultTTL: time.Minute,
	})

	query := map[string]interface{}{"limit": 10}
	cache.Set("users", query, "result")

	_, found := cache.Get("users", query)
	if found {
		t.Error("expected cache to be disabled")
	}
}

func TestQueryCache_WithCache(t *testing.T) {
	cache := NewQueryCache(&QueryCacheConfig{
		Enabled:    true,
		MaxSize:    100,
		DefaultTTL: time.Minute,
	})

	callCount := 0
	query := map[string]interface{}{"id": 1}

	fn := func() (interface{}, error) {
		callCount++
		return "result", nil
	}

	// First call - should execute function
	result1, err := cache.WithCache("users", query, fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Error("expected function to be called once")
	}
	if result1 != "result" {
		t.Errorf("expected result, got %v", result1)
	}

	// Second call - should use cache
	result2, err := cache.WithCache("users", query, fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Error("expected function to NOT be called again")
	}
	if result2 != "result" {
		t.Errorf("expected result, got %v", result2)
	}
}

func TestCacheKey(t *testing.T) {
	key1 := CacheKey("users", map[string]interface{}{"id": 1})
	key2 := CacheKey("users", map[string]interface{}{"id": 1})
	key3 := CacheKey("users", map[string]interface{}{"id": 2})
	key4 := CacheKey("orders", map[string]interface{}{"id": 1})

	if key1 != key2 {
		t.Error("expected same queries to produce same key")
	}
	if key1 == key3 {
		t.Error("expected different queries to produce different keys")
	}
	if key1 == key4 {
		t.Error("expected different collections to produce different keys")
	}
}

// Benchmarks
func BenchmarkInMemoryCache_Set(b *testing.B) {
	cache := NewInMemoryCache(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(string(rune(i%1000)), i, time.Minute)
	}
}

func BenchmarkInMemoryCache_Get(b *testing.B) {
	cache := NewInMemoryCache(10000)

	// Pre-populate
	for i := 0; i < 1000; i++ {
		cache.Set(string(rune(i)), i, time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(string(rune(i % 1000)))
	}
}

func BenchmarkInMemoryCache_Concurrent(b *testing.B) {
	cache := NewInMemoryCache(10000)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune(i % 1000))
			if i%2 == 0 {
				cache.Set(key, i, time.Minute)
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}

func BenchmarkQueryCache_WithCache(b *testing.B) {
	cache := NewQueryCache(&QueryCacheConfig{
		Enabled:    true,
		MaxSize:    10000,
		DefaultTTL: time.Minute,
	})

	fn := func() (interface{}, error) {
		return "result", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := map[string]interface{}{"id": i % 100}
		cache.WithCache("users", query, fn)
	}
}
