package cache

import (
	"container/list"
	"context"
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

// AdvancedCacheConfig holds advanced cache configuration
type AdvancedCacheConfig struct {
	// Enable caching
	Enabled bool `json:"enabled"`

	// Maximum number of cached entries
	MaxSize int `json:"max_size"`

	// Default TTL for cached entries
	DefaultTTL time.Duration `json:"default_ttl"`

	// Cleanup interval for expired entries
	CleanupInterval time.Duration `json:"cleanup_interval"`

	// Per-collection cache settings
	Collections map[string]CollectionCacheSettings `json:"collections,omitempty"`

	// Enable cache statistics
	StatsEnabled bool `json:"stats_enabled"`

	// Invalidate on mutation
	InvalidateOnMutation bool `json:"invalidate_on_mutation"`
}

// CollectionCacheSettings holds per-collection cache settings
type CollectionCacheSettings struct {
	// Enable caching for this collection
	Enabled bool `json:"enabled"`

	// Custom TTL for this collection
	TTL time.Duration `json:"ttl"`

	// Maximum entries for this collection
	MaxSize int `json:"max_size"`
}

// DefaultAdvancedCacheConfig returns default advanced cache configuration
func DefaultAdvancedCacheConfig() *AdvancedCacheConfig {
	return &AdvancedCacheConfig{
		Enabled:              true,
		MaxSize:              10000,
		DefaultTTL:           time.Minute * 5,
		CleanupInterval:      time.Minute * 1,
		StatsEnabled:         true,
		InvalidateOnMutation: true,
		Collections:          make(map[string]CollectionCacheSettings),
	}
}

// AdvancedCache provides an enhanced caching system with per-collection settings
type AdvancedCache struct {
	config      *AdvancedCacheConfig
	cache       *InMemoryCache
	collections map[string]*InMemoryCache
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewAdvancedCache creates a new advanced cache
func NewAdvancedCache(cfg *AdvancedCacheConfig) *AdvancedCache {
	if cfg == nil {
		cfg = DefaultAdvancedCacheConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	ac := &AdvancedCache{
		config:      cfg,
		cache:       NewInMemoryCache(cfg.MaxSize),
		collections: make(map[string]*InMemoryCache),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize per-collection caches
	for collection, settings := range cfg.Collections {
		maxSize := settings.MaxSize
		if maxSize <= 0 {
			maxSize = cfg.MaxSize / 10 // Default to 10% of total size
		}
		ac.collections[collection] = NewInMemoryCache(maxSize)
	}

	// Start cleanup goroutine
	if cfg.CleanupInterval > 0 {
		go ac.cleanupLoop()
	}

	return ac
}

// cleanupLoop periodically cleans up expired entries
func (ac *AdvancedCache) cleanupLoop() {
	ticker := time.NewTicker(ac.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ac.ctx.Done():
			return
		case <-ticker.C:
			ac.cleanup()
		}
	}
}

// cleanup removes expired entries from all caches
func (ac *AdvancedCache) cleanup() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Cleanup main cache
	ac.cleanupCache(ac.cache)

	// Cleanup per-collection caches
	for _, cache := range ac.collections {
		ac.cleanupCache(cache)
	}
}

// cleanupCache removes expired entries from a single cache
func (ac *AdvancedCache) cleanupCache(cache *InMemoryCache) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	now := time.Now()
	var toRemove []*cacheItem

	for _, item := range cache.items {
		if now.After(item.expiresAt) {
			toRemove = append(toRemove, item)
		}
	}

	for _, item := range toRemove {
		cache.lru.Remove(item.element)
		delete(cache.items, item.key)
		cache.evictions++
	}
}

// Get retrieves a value from the cache
func (ac *AdvancedCache) Get(collection string, key string) (interface{}, bool) {
	if !ac.config.Enabled {
		return nil, false
	}

	// Check collection settings
	if settings, ok := ac.config.Collections[collection]; ok && !settings.Enabled {
		return nil, false
	}

	// Try collection-specific cache first
	if cache, ok := ac.collections[collection]; ok {
		if val, found := cache.Get(key); found {
			return val, true
		}
	}

	// Fall back to main cache
	return ac.cache.Get(key)
}

// Set stores a value in the cache
func (ac *AdvancedCache) Set(collection string, key string, value interface{}) {
	if !ac.config.Enabled {
		return
	}

	// Check collection settings
	settings, hasSettings := ac.config.Collections[collection]
	if hasSettings && !settings.Enabled {
		return
	}

	// Determine TTL
	ttl := ac.config.DefaultTTL
	if hasSettings && settings.TTL > 0 {
		ttl = settings.TTL
	}

	// Use collection-specific cache if available
	if cache, ok := ac.collections[collection]; ok {
		cache.Set(key, value, ttl)
		return
	}

	// Fall back to main cache
	ac.cache.Set(key, value, ttl)
}

// SetWithTTL stores a value with a custom TTL
func (ac *AdvancedCache) SetWithTTL(collection string, key string, value interface{}, ttl time.Duration) {
	if !ac.config.Enabled {
		return
	}

	// Check collection settings
	if settings, ok := ac.config.Collections[collection]; ok && !settings.Enabled {
		return
	}

	// Use collection-specific cache if available
	if cache, ok := ac.collections[collection]; ok {
		cache.Set(key, value, ttl)
		return
	}

	// Fall back to main cache
	ac.cache.Set(key, value, ttl)
}

// Delete removes a value from the cache
func (ac *AdvancedCache) Delete(collection string, key string) {
	if cache, ok := ac.collections[collection]; ok {
		cache.Delete(key)
	}
	ac.cache.Delete(key)
}

// InvalidateCollection removes all cached entries for a collection
func (ac *AdvancedCache) InvalidateCollection(collection string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if cache, ok := ac.collections[collection]; ok {
		cache.Clear()
	}
}

// Clear removes all cached entries
func (ac *AdvancedCache) Clear() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.cache.Clear()
	for _, cache := range ac.collections {
		cache.Clear()
	}
}

// Stats returns aggregated cache statistics
func (ac *AdvancedCache) Stats() AdvancedCacheStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	mainStats := ac.cache.Stats()
	collectionStats := make(map[string]CacheStats)

	var totalHits, totalMisses, totalEvictions int64
	var totalSize int

	totalHits = mainStats.Hits
	totalMisses = mainStats.Misses
	totalEvictions = mainStats.Evictions
	totalSize = mainStats.Size

	for name, cache := range ac.collections {
		stats := cache.Stats()
		collectionStats[name] = stats
		totalHits += stats.Hits
		totalMisses += stats.Misses
		totalEvictions += stats.Evictions
		totalSize += stats.Size
	}

	totalRequests := totalHits + totalMisses
	hitRate := float64(0)
	if totalRequests > 0 {
		hitRate = float64(totalHits) / float64(totalRequests)
	}

	return AdvancedCacheStats{
		Enabled:         ac.config.Enabled,
		TotalHits:       totalHits,
		TotalMisses:     totalMisses,
		TotalEvictions:  totalEvictions,
		TotalSize:       totalSize,
		HitRate:         hitRate,
		MainCache:       mainStats,
		Collections:     collectionStats,
		CollectionCount: len(ac.collections),
	}
}

// AdvancedCacheStats holds advanced cache statistics
type AdvancedCacheStats struct {
	Enabled         bool                  `json:"enabled"`
	TotalHits       int64                 `json:"total_hits"`
	TotalMisses     int64                 `json:"total_misses"`
	TotalEvictions  int64                 `json:"total_evictions"`
	TotalSize       int                   `json:"total_size"`
	HitRate         float64               `json:"hit_rate"`
	MainCache       CacheStats            `json:"main_cache"`
	Collections     map[string]CacheStats `json:"collections"`
	CollectionCount int                   `json:"collection_count"`
}

// IsEnabled returns whether the cache is enabled
func (ac *AdvancedCache) IsEnabled() bool {
	return ac.config.Enabled
}

// Close shuts down the cache and stops the cleanup goroutine
func (ac *AdvancedCache) Close() {
	ac.cancel()
}

// CacheKey generates a cache key from collection and query
func (ac *AdvancedCache) CacheKey(collection string, query interface{}) string {
	return CacheKey(collection, query)
}

// GetQuery retrieves a cached query result
func (ac *AdvancedCache) GetQuery(collection string, query interface{}) (interface{}, bool) {
	key := ac.CacheKey(collection, query)
	return ac.Get(collection, key)
}

// SetQuery caches a query result
func (ac *AdvancedCache) SetQuery(collection string, query interface{}, result interface{}) {
	key := ac.CacheKey(collection, query)
	ac.Set(collection, key, result)
}

// WithQuery wraps a query function with caching
func (ac *AdvancedCache) WithQuery(collection string, query interface{}, fn CachedQuery) (interface{}, error) {
	// Try to get from cache
	if result, found := ac.GetQuery(collection, query); found {
		return result, nil
	}

	// Execute the query
	result, err := fn()
	if err != nil {
		return nil, err
	}

	// Cache the result
	ac.SetQuery(collection, query, result)

	return result, nil
}

// ShouldInvalidateOnMutation returns whether cache should be invalidated on mutations
func (ac *AdvancedCache) ShouldInvalidateOnMutation() bool {
	return ac.config.InvalidateOnMutation
}

// ParseDuration parses a duration string like "5m", "1h", etc.
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// ConfigFromMap creates cache config from a map (for JSON deserialization)
func ConfigFromMap(m map[string]interface{}) (*AdvancedCacheConfig, error) {
	cfg := DefaultAdvancedCacheConfig()

	if enabled, ok := m["enabled"].(bool); ok {
		cfg.Enabled = enabled
	}

	if maxSize, ok := m["max_size"].(float64); ok {
		cfg.MaxSize = int(maxSize)
	}

	if ttlStr, ok := m["default_ttl"].(string); ok {
		if ttl, err := ParseDuration(ttlStr); err == nil {
			cfg.DefaultTTL = ttl
		}
	}

	if cleanupStr, ok := m["cleanup_interval"].(string); ok {
		if interval, err := ParseDuration(cleanupStr); err == nil {
			cfg.CleanupInterval = interval
		}
	}

	if statsEnabled, ok := m["stats_enabled"].(bool); ok {
		cfg.StatsEnabled = statsEnabled
	}

	if invalidation, ok := m["invalidation"].(map[string]interface{}); ok {
		if onMutation, ok := invalidation["on_mutation"].(bool); ok {
			cfg.InvalidateOnMutation = onMutation
		}
	}

	if collections, ok := m["collections"].(map[string]interface{}); ok {
		for name, collCfg := range collections {
			if collMap, ok := collCfg.(map[string]interface{}); ok {
				settings := CollectionCacheSettings{Enabled: true}

				if enabled, ok := collMap["enabled"].(bool); ok {
					settings.Enabled = enabled
				}

				if ttlStr, ok := collMap["ttl"].(string); ok {
					if ttl, err := ParseDuration(ttlStr); err == nil {
						settings.TTL = ttl
					}
				}

				if maxSize, ok := collMap["max_size"].(float64); ok {
					settings.MaxSize = int(maxSize)
				}

				cfg.Collections[name] = settings
			}
		}
	}

	return cfg, nil
}
