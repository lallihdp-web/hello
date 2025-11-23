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

// Advanced Cache Tests

func TestAdvancedCache_Basic(t *testing.T) {
	cfg := DefaultAdvancedCacheConfig()
	cfg.CleanupInterval = 0 // Disable cleanup for tests
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	// Test basic set and get
	cache.Set("users", "key1", "value1")
	val, found := cache.Get("users", "key1")
	if !found {
		t.Fatal("expected to find cached value")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestAdvancedCache_Disabled(t *testing.T) {
	cfg := &AdvancedCacheConfig{
		Enabled: false,
	}
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	cache.Set("users", "key1", "value1")
	_, found := cache.Get("users", "key1")
	if found {
		t.Error("expected cache to be disabled")
	}
}

func TestAdvancedCache_PerCollectionSettings(t *testing.T) {
	cfg := &AdvancedCacheConfig{
		Enabled:         true,
		MaxSize:         1000,
		DefaultTTL:      time.Minute,
		CleanupInterval: 0, // Disable cleanup for tests
		Collections: map[string]CollectionCacheSettings{
			"users": {
				Enabled: true,
				TTL:     time.Minute * 10,
				MaxSize: 100,
			},
			"disabled_collection": {
				Enabled: false,
			},
		},
	}
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	// Users collection should work
	cache.Set("users", "key1", "value1")
	val, found := cache.Get("users", "key1")
	if !found {
		t.Fatal("expected to find cached value in users collection")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}

	// Disabled collection should not cache
	cache.Set("disabled_collection", "key1", "value1")
	_, found = cache.Get("disabled_collection", "key1")
	if found {
		t.Error("expected disabled_collection to not cache values")
	}

	// Other collections should use main cache
	cache.Set("orders", "key1", "value1")
	val, found = cache.Get("orders", "key1")
	if !found {
		t.Fatal("expected to find cached value in orders collection")
	}
}

func TestAdvancedCache_Stats(t *testing.T) {
	cfg := &AdvancedCacheConfig{
		Enabled:         true,
		MaxSize:         1000,
		DefaultTTL:      time.Minute,
		CleanupInterval: 0,
		Collections: map[string]CollectionCacheSettings{
			"users": {
				Enabled: true,
				MaxSize: 100,
			},
		},
	}
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	// Generate some hits and misses
	cache.Set("users", "key1", "value1")
	cache.Get("users", "key1") // Hit
	cache.Get("users", "key1") // Hit
	cache.Get("users", "key2") // Miss

	cache.Set("orders", "key1", "value1")
	cache.Get("orders", "key1") // Hit
	cache.Get("orders", "key2") // Miss

	stats := cache.Stats()
	if !stats.Enabled {
		t.Error("expected cache to be enabled")
	}
	if stats.TotalHits < 3 {
		t.Errorf("expected at least 3 hits, got %d", stats.TotalHits)
	}
	if stats.TotalMisses < 2 {
		t.Errorf("expected at least 2 misses, got %d", stats.TotalMisses)
	}
	if stats.CollectionCount != 1 {
		t.Errorf("expected 1 collection cache, got %d", stats.CollectionCount)
	}
}

func TestAdvancedCache_InvalidateCollection(t *testing.T) {
	cfg := &AdvancedCacheConfig{
		Enabled:         true,
		MaxSize:         1000,
		DefaultTTL:      time.Minute,
		CleanupInterval: 0,
		Collections: map[string]CollectionCacheSettings{
			"users": {
				Enabled: true,
				MaxSize: 100,
			},
		},
	}
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	cache.Set("users", "key1", "value1")
	cache.Set("users", "key2", "value2")

	// Verify entries exist
	_, found := cache.Get("users", "key1")
	if !found {
		t.Fatal("expected to find key1 before invalidation")
	}

	// Invalidate
	cache.InvalidateCollection("users")

	// Verify entries are gone
	_, found = cache.Get("users", "key1")
	if found {
		t.Error("expected key1 to be invalidated")
	}
}

func TestAdvancedCache_Clear(t *testing.T) {
	cfg := &AdvancedCacheConfig{
		Enabled:         true,
		MaxSize:         1000,
		DefaultTTL:      time.Minute,
		CleanupInterval: 0,
		Collections: map[string]CollectionCacheSettings{
			"users": {
				Enabled: true,
				MaxSize: 100,
			},
		},
	}
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	cache.Set("users", "key1", "value1")
	cache.Set("orders", "key1", "value1")

	cache.Clear()

	stats := cache.Stats()
	if stats.TotalSize != 0 {
		t.Errorf("expected empty cache after clear, got size %d", stats.TotalSize)
	}
}

func TestAdvancedCache_WithQuery(t *testing.T) {
	cfg := DefaultAdvancedCacheConfig()
	cfg.CleanupInterval = 0
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	callCount := 0
	query := map[string]interface{}{"id": 1}

	fn := func() (interface{}, error) {
		callCount++
		return "result", nil
	}

	// First call - should execute function
	result1, err := cache.WithQuery("users", query, fn)
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
	result2, err := cache.WithQuery("users", query, fn)
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

func TestAdvancedCache_ShouldInvalidateOnMutation(t *testing.T) {
	cfg := &AdvancedCacheConfig{
		Enabled:              true,
		InvalidateOnMutation: true,
	}
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	if !cache.ShouldInvalidateOnMutation() {
		t.Error("expected InvalidateOnMutation to be true")
	}

	cfg2 := &AdvancedCacheConfig{
		Enabled:              true,
		InvalidateOnMutation: false,
	}
	cache2 := NewAdvancedCache(cfg2)
	defer cache2.Close()

	if cache2.ShouldInvalidateOnMutation() {
		t.Error("expected InvalidateOnMutation to be false")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		hasError bool
	}{
		{"5m", time.Minute * 5, false},
		{"1h", time.Hour, false},
		{"30s", time.Second * 30, false},
		{"", 0, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseDuration(tt.input)
			if tt.hasError && err == nil {
				t.Error("expected error")
			}
			if !tt.hasError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if d != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, d)
			}
		})
	}
}

func TestConfigFromMap(t *testing.T) {
	m := map[string]interface{}{
		"enabled":          true,
		"max_size":         float64(5000),
		"default_ttl":      "10m",
		"cleanup_interval": "2m",
		"stats_enabled":    true,
		"invalidation": map[string]interface{}{
			"on_mutation": true,
		},
		"collections": map[string]interface{}{
			"users": map[string]interface{}{
				"enabled":  true,
				"ttl":      "15m",
				"max_size": float64(500),
			},
		},
	}

	cfg, err := ConfigFromMap(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Enabled {
		t.Error("expected cache to be enabled")
	}
	if cfg.MaxSize != 5000 {
		t.Errorf("expected max_size 5000, got %d", cfg.MaxSize)
	}
	if cfg.DefaultTTL != time.Minute*10 {
		t.Errorf("expected default_ttl 10m, got %v", cfg.DefaultTTL)
	}
	if cfg.CleanupInterval != time.Minute*2 {
		t.Errorf("expected cleanup_interval 2m, got %v", cfg.CleanupInterval)
	}
	if !cfg.InvalidateOnMutation {
		t.Error("expected invalidate_on_mutation to be true")
	}

	userCfg, ok := cfg.Collections["users"]
	if !ok {
		t.Fatal("expected users collection config")
	}
	if !userCfg.Enabled {
		t.Error("expected users cache to be enabled")
	}
	if userCfg.TTL != time.Minute*15 {
		t.Errorf("expected users TTL 15m, got %v", userCfg.TTL)
	}
	if userCfg.MaxSize != 500 {
		t.Errorf("expected users max_size 500, got %d", userCfg.MaxSize)
	}
}

// Advanced Cache Benchmarks

func BenchmarkAdvancedCache_Set(b *testing.B) {
	cfg := DefaultAdvancedCacheConfig()
	cfg.CleanupInterval = 0
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("users", string(rune(i%1000)), i)
	}
}

func BenchmarkAdvancedCache_Get(b *testing.B) {
	cfg := DefaultAdvancedCacheConfig()
	cfg.CleanupInterval = 0
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		cache.Set("users", string(rune(i)), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("users", string(rune(i%1000)))
	}
}

func BenchmarkAdvancedCache_Concurrent(b *testing.B) {
	cfg := DefaultAdvancedCacheConfig()
	cfg.CleanupInterval = 0
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune(i % 1000))
			if i%2 == 0 {
				cache.Set("users", key, i)
			} else {
				cache.Get("users", key)
			}
			i++
		}
	})
}

func BenchmarkAdvancedCache_WithQuery(b *testing.B) {
	cfg := DefaultAdvancedCacheConfig()
	cfg.CleanupInterval = 0
	cache := NewAdvancedCache(cfg)
	defer cache.Close()

	fn := func() (interface{}, error) {
		return "result", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := map[string]interface{}{"id": i % 100}
		cache.WithQuery("users", query, fn)
	}
}
