package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Cache provides query result caching
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
	Stats() CacheStats
}

// CacheStats provides cache statistics
type CacheStats struct {
	Hits       int64 `json:"hits"`
	Misses     int64 `json:"misses"`
	Size       int   `json:"size"`
	MaxSize    int   `json:"max_size"`
	Evictions  int64 `json:"evictions"`
	HitRate    float64 `json:"hit_rate"`
}

// InMemoryCache is an LRU cache implementation
type InMemoryCache struct {
	maxSize    int
	items      map[string]*cacheItem
	lru        *list.List
	mu         sync.RWMutex
	hits       int64
	misses     int64
	evictions  int64
}

type cacheItem struct {
	key       string
	value     interface{}
	expiresAt time.Time
	element   *list.Element
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache(maxSize int) *InMemoryCache {
	if maxSize <= 0 {
		maxSize = 1000
	}

	return &InMemoryCache{
		maxSize: maxSize,
		items:   make(map[string]*cacheItem),
		lru:     list.New(),
	}
}

// Get retrieves a value from the cache
func (c *InMemoryCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check expiration
	if time.Now().After(item.expiresAt) {
		c.removeItem(item)
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(item.element)
	c.hits++

	return item.value, true
}

// Set stores a value in the cache
func (c *InMemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if item already exists
	if item, exists := c.items[key]; exists {
		item.value = value
		item.expiresAt = time.Now().Add(ttl)
		c.lru.MoveToFront(item.element)
		return
	}

	// Evict if necessary
	for len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	// Add new item
	item := &cacheItem{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	item.element = c.lru.PushFront(item)
	c.items[key] = item
}

// Delete removes a value from the cache
func (c *InMemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		c.removeItem(item)
	}
}

// Clear removes all items from the cache
func (c *InMemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.lru = list.New()
}

// Stats returns cache statistics
func (c *InMemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		Size:      len(c.items),
		MaxSize:   c.maxSize,
		Evictions: c.evictions,
		HitRate:   hitRate,
	}
}

func (c *InMemoryCache) removeItem(item *cacheItem) {
	c.lru.Remove(item.element)
	delete(c.items, item.key)
}

func (c *InMemoryCache) evictOldest() {
	oldest := c.lru.Back()
	if oldest != nil {
		item := oldest.Value.(*cacheItem)
		c.removeItem(item)
		c.evictions++
	}
}

// QueryCache wraps Cache with query-specific functionality
type QueryCache struct {
	cache       Cache
	enabled     bool
	defaultTTL  time.Duration
	mu          sync.RWMutex
}

// QueryCacheConfig holds query cache configuration
type QueryCacheConfig struct {
	Enabled    bool          `json:"enabled"`
	MaxSize    int           `json:"max_size"`
	DefaultTTL time.Duration `json:"default_ttl"`
}

// DefaultQueryCacheConfig returns default cache configuration
func DefaultQueryCacheConfig() *QueryCacheConfig {
	return &QueryCacheConfig{
		Enabled:    true,
		MaxSize:    1000,
		DefaultTTL: time.Minute * 5,
	}
}

// NewQueryCache creates a new query cache
func NewQueryCache(cfg *QueryCacheConfig) *QueryCache {
	if cfg == nil {
		cfg = DefaultQueryCacheConfig()
	}

	return &QueryCache{
		cache:      NewInMemoryCache(cfg.MaxSize),
		enabled:    cfg.Enabled,
		defaultTTL: cfg.DefaultTTL,
	}
}

// CacheKey generates a cache key for a query
func CacheKey(collection string, query interface{}) string {
	data, _ := json.Marshal(query)
	hash := sha256.Sum256(append([]byte(collection), data...))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached query result
func (qc *QueryCache) Get(collection string, query interface{}) (interface{}, bool) {
	if !qc.enabled {
		return nil, false
	}

	key := CacheKey(collection, query)
	return qc.cache.Get(key)
}

// Set caches a query result
func (qc *QueryCache) Set(collection string, query interface{}, result interface{}) {
	if !qc.enabled {
		return
	}

	key := CacheKey(collection, query)
	qc.cache.Set(key, result, qc.defaultTTL)
}

// SetWithTTL caches a query result with a specific TTL
func (qc *QueryCache) SetWithTTL(collection string, query interface{}, result interface{}, ttl time.Duration) {
	if !qc.enabled {
		return
	}

	key := CacheKey(collection, query)
	qc.cache.Set(key, result, ttl)
}

// Invalidate removes a cached query result
func (qc *QueryCache) Invalidate(collection string, query interface{}) {
	key := CacheKey(collection, query)
	qc.cache.Delete(key)
}

// InvalidateCollection removes all cached results for a collection
func (qc *QueryCache) InvalidateCollection(collection string) {
	// For a full implementation, we'd need to track keys by collection
	// For now, we clear the entire cache
	qc.cache.Clear()
}

// Clear removes all cached results
func (qc *QueryCache) Clear() {
	qc.cache.Clear()
}

// Enable enables the cache
func (qc *QueryCache) Enable() {
	qc.mu.Lock()
	qc.enabled = true
	qc.mu.Unlock()
}

// Disable disables the cache
func (qc *QueryCache) Disable() {
	qc.mu.Lock()
	qc.enabled = false
	qc.mu.Unlock()
}

// IsEnabled returns whether the cache is enabled
func (qc *QueryCache) IsEnabled() bool {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	return qc.enabled
}

// Stats returns cache statistics
func (qc *QueryCache) Stats() CacheStats {
	return qc.cache.Stats()
}

// CachedQuery represents a function that can be cached
type CachedQuery func() (interface{}, error)

// WithCache wraps a query function with caching
func (qc *QueryCache) WithCache(collection string, query interface{}, fn CachedQuery) (interface{}, error) {
	// Try to get from cache
	if result, found := qc.Get(collection, query); found {
		return result, nil
	}

	// Execute the query
	result, err := fn()
	if err != nil {
		return nil, err
	}

	// Cache the result
	qc.Set(collection, query, result)

	return result, nil
}
