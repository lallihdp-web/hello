package middleware

import (
	"container/list"
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// LeakyBucketConfig holds configuration for leaky bucket rate limiter
type LeakyBucketConfig struct {
	// Enable the rate limiter
	Enabled bool `json:"enabled"`

	// BucketSize is the maximum number of requests that can be processed
	BucketSize int `json:"bucket_size"`

	// LeakRate is the number of requests processed per second
	LeakRate float64 `json:"leak_rate"`

	// QueueSize is the buffer size for requests waiting to be processed
	// Set to 0 to disable queuing (reject immediately when bucket is full)
	QueueSize int `json:"queue_size"`

	// QueueTimeout is how long a request can wait in the queue
	QueueTimeout time.Duration `json:"queue_timeout"`

	// Per-role configurations
	RoleConfigs map[string]*LeakyBucketRoleConfig `json:"role_configs"`

	// Per-endpoint configurations
	EndpointConfigs map[string]*LeakyBucketRoleConfig `json:"endpoint_configs"`
}

// LeakyBucketRoleConfig holds per-role/endpoint configuration
type LeakyBucketRoleConfig struct {
	BucketSize   int           `json:"bucket_size"`
	LeakRate     float64       `json:"leak_rate"`
	QueueSize    int           `json:"queue_size"`
	QueueTimeout time.Duration `json:"queue_timeout"`
}

// DefaultLeakyBucketConfig returns default configuration
func DefaultLeakyBucketConfig() *LeakyBucketConfig {
	return &LeakyBucketConfig{
		Enabled:      true,
		BucketSize:   100,              // 100 concurrent requests
		LeakRate:     10,               // 10 requests per second
		QueueSize:    50,               // Buffer 50 additional requests
		QueueTimeout: 30 * time.Second, // Wait max 30 seconds in queue
		RoleConfigs: map[string]*LeakyBucketRoleConfig{
			"admin": {
				BucketSize:   1000,
				LeakRate:     100,
				QueueSize:    500,
				QueueTimeout: 60 * time.Second,
			},
			"user": {
				BucketSize:   100,
				LeakRate:     10,
				QueueSize:    50,
				QueueTimeout: 30 * time.Second,
			},
			"anonymous": {
				BucketSize:   20,
				LeakRate:     2,
				QueueSize:    10,
				QueueTimeout: 10 * time.Second,
			},
		},
	}
}

// LeakyBucket implements the leaky bucket algorithm
type LeakyBucket struct {
	config  *LeakyBucketConfig
	buckets map[string]*leakyBucketInstance
	mu      sync.RWMutex
}

// leakyBucketInstance represents a single bucket for a client
type leakyBucketInstance struct {
	// Configuration
	bucketSize   int
	leakRate     float64
	queueSize    int
	queueTimeout time.Duration

	// State
	currentLevel float64   // Current water level in bucket
	lastLeak     time.Time // Last time we leaked
	queue        *requestQueue
	mu           sync.Mutex

	// Metrics
	totalRequests   int64
	droppedRequests int64
	queuedRequests  int64
}

// requestQueue manages queued requests
type requestQueue struct {
	items    *list.List
	maxSize  int
	mu       sync.Mutex
	notEmpty *sync.Cond
}

// queuedRequest represents a request waiting in the queue
type queuedRequest struct {
	addedAt  time.Time
	resultCh chan bool
}

// NewLeakyBucket creates a new leaky bucket rate limiter
func NewLeakyBucket(config *LeakyBucketConfig) *LeakyBucket {
	if config == nil {
		config = DefaultLeakyBucketConfig()
	}

	lb := &LeakyBucket{
		config:  config,
		buckets: make(map[string]*leakyBucketInstance),
	}

	// Start the leak goroutine
	go lb.leakLoop()

	return lb
}

// leakLoop continuously leaks from all buckets
func (lb *LeakyBucket) leakLoop() {
	ticker := time.NewTicker(100 * time.Millisecond) // Leak every 100ms
	defer ticker.Stop()

	for range ticker.C {
		lb.leakAll()
	}
}

// leakAll leaks from all active buckets
func (lb *LeakyBucket) leakAll() {
	lb.mu.RLock()
	buckets := make([]*leakyBucketInstance, 0, len(lb.buckets))
	for _, b := range lb.buckets {
		buckets = append(buckets, b)
	}
	lb.mu.RUnlock()

	for _, bucket := range buckets {
		bucket.leak()
	}
}

// getBucket gets or creates a bucket for the given key
func (lb *LeakyBucket) getBucket(key string, roleConfig *LeakyBucketRoleConfig) *leakyBucketInstance {
	lb.mu.RLock()
	bucket, exists := lb.buckets[key]
	lb.mu.RUnlock()

	if exists {
		return bucket
	}

	// Create new bucket
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Double-check after acquiring write lock
	if bucket, exists = lb.buckets[key]; exists {
		return bucket
	}

	// Use role config or defaults
	bucketSize := lb.config.BucketSize
	leakRate := lb.config.LeakRate
	queueSize := lb.config.QueueSize
	queueTimeout := lb.config.QueueTimeout

	if roleConfig != nil {
		bucketSize = roleConfig.BucketSize
		leakRate = roleConfig.LeakRate
		queueSize = roleConfig.QueueSize
		queueTimeout = roleConfig.QueueTimeout
	}

	bucket = &leakyBucketInstance{
		bucketSize:   bucketSize,
		leakRate:     leakRate,
		queueSize:    queueSize,
		queueTimeout: queueTimeout,
		currentLevel: 0,
		lastLeak:     time.Now(),
		queue:        newRequestQueue(queueSize),
	}

	lb.buckets[key] = bucket
	return bucket
}

// Allow checks if a request is allowed (non-blocking)
func (lb *LeakyBucket) Allow(key string) bool {
	if !lb.config.Enabled {
		return true
	}

	bucket := lb.getBucket(key, nil)
	return bucket.tryAdd()
}

// AllowWithQueue checks if a request is allowed, waiting in queue if necessary
func (lb *LeakyBucket) AllowWithQueue(ctx context.Context, key string) (bool, error) {
	if !lb.config.Enabled {
		return true, nil
	}

	bucket := lb.getBucket(key, nil)
	return bucket.addWithQueue(ctx)
}

// AllowByRole checks with role-specific configuration
func (lb *LeakyBucket) AllowByRole(role, identifier string) bool {
	if !lb.config.Enabled {
		return true
	}

	roleConfig := lb.config.RoleConfigs[role]
	key := "role:" + role + ":" + identifier
	bucket := lb.getBucket(key, roleConfig)
	return bucket.tryAdd()
}

// AllowByRoleWithQueue checks with role-specific configuration, with queuing
func (lb *LeakyBucket) AllowByRoleWithQueue(ctx context.Context, role, identifier string) (bool, error) {
	if !lb.config.Enabled {
		return true, nil
	}

	roleConfig := lb.config.RoleConfigs[role]
	key := "role:" + role + ":" + identifier
	bucket := lb.getBucket(key, roleConfig)
	return bucket.addWithQueue(ctx)
}

// AllowByEndpoint checks with endpoint-specific configuration
func (lb *LeakyBucket) AllowByEndpoint(endpoint, identifier string) bool {
	if !lb.config.Enabled {
		return true
	}

	endpointConfig := lb.config.EndpointConfigs[endpoint]
	key := "endpoint:" + endpoint + ":" + identifier
	bucket := lb.getBucket(key, endpointConfig)
	return bucket.tryAdd()
}

// tryAdd tries to add a request to the bucket (non-blocking)
func (b *leakyBucketInstance) tryAdd() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalRequests++

	// Leak first
	b.leakLocked()

	// Check if bucket has space
	if b.currentLevel < float64(b.bucketSize) {
		b.currentLevel++
		return true
	}

	b.droppedRequests++
	return false
}

// addWithQueue tries to add a request, queuing if bucket is full
func (b *leakyBucketInstance) addWithQueue(ctx context.Context) (bool, error) {
	b.mu.Lock()
	b.totalRequests++

	// Leak first
	b.leakLocked()

	// Check if bucket has space
	if b.currentLevel < float64(b.bucketSize) {
		b.currentLevel++
		b.mu.Unlock()
		return true, nil
	}

	// Try to queue the request
	if b.queueSize == 0 {
		b.droppedRequests++
		b.mu.Unlock()
		return false, nil
	}

	b.queuedRequests++
	b.mu.Unlock()

	// Add to queue and wait
	return b.queue.waitForSlot(ctx, b.queueTimeout)
}

// leak removes water from the bucket based on elapsed time
func (b *leakyBucketInstance) leak() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.leakLocked()
}

// leakLocked removes water (must hold lock)
func (b *leakyBucketInstance) leakLocked() {
	now := time.Now()
	elapsed := now.Sub(b.lastLeak).Seconds()
	leaked := elapsed * b.leakRate

	b.currentLevel -= leaked
	if b.currentLevel < 0 {
		b.currentLevel = 0
	}
	b.lastLeak = now

	// Process queued requests if there's space
	if b.queue != nil && b.currentLevel < float64(b.bucketSize) {
		slotsAvailable := int(float64(b.bucketSize) - b.currentLevel)
		b.queue.releaseSlots(slotsAvailable)
	}
}

// GetStats returns statistics for a bucket
func (lb *LeakyBucket) GetStats(key string) *LeakyBucketStats {
	lb.mu.RLock()
	bucket, exists := lb.buckets[key]
	lb.mu.RUnlock()

	if !exists {
		return nil
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	return &LeakyBucketStats{
		CurrentLevel:    bucket.currentLevel,
		BucketSize:      bucket.bucketSize,
		QueueLength:     bucket.queue.length(),
		QueueSize:       bucket.queueSize,
		TotalRequests:   bucket.totalRequests,
		DroppedRequests: bucket.droppedRequests,
		QueuedRequests:  bucket.queuedRequests,
		LeakRate:        bucket.leakRate,
	}
}

// LeakyBucketStats holds statistics for a bucket
type LeakyBucketStats struct {
	CurrentLevel    float64 `json:"current_level"`
	BucketSize      int     `json:"bucket_size"`
	QueueLength     int     `json:"queue_length"`
	QueueSize       int     `json:"queue_size"`
	TotalRequests   int64   `json:"total_requests"`
	DroppedRequests int64   `json:"dropped_requests"`
	QueuedRequests  int64   `json:"queued_requests"`
	LeakRate        float64 `json:"leak_rate"`
}

// Cleanup removes inactive buckets
func (lb *LeakyBucket) Cleanup(maxIdle time.Duration) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	now := time.Now()
	for key, bucket := range lb.buckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastLeak) > maxIdle && bucket.currentLevel == 0 {
			delete(lb.buckets, key)
		}
		bucket.mu.Unlock()
	}
}

// Middleware returns an HTTP middleware for leaky bucket rate limiting
func (lb *LeakyBucket) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !lb.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get identifier
		identifier := r.RemoteAddr
		if userID := r.Header.Get("X-User-Id"); userID != "" {
			identifier = userID
		}

		// Get role
		role := r.Header.Get("X-Role")
		if role == "" {
			role = "anonymous"
		}

		// Check rate limit
		if !lb.AllowByRole(role, identifier) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Type", "leaky-bucket")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// MiddlewareWithQueue returns middleware that queues requests instead of rejecting
func (lb *LeakyBucket) MiddlewareWithQueue(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !lb.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get identifier
		identifier := r.RemoteAddr
		if userID := r.Header.Get("X-User-Id"); userID != "" {
			identifier = userID
		}

		// Get role
		role := r.Header.Get("X-Role")
		if role == "" {
			role = "anonymous"
		}

		// Check rate limit with queuing
		allowed, err := lb.AllowByRoleWithQueue(r.Context(), role, identifier)
		if err != nil {
			// Context cancelled (client disconnected)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Type", "leaky-bucket")
			http.Error(w, "Rate limit exceeded - queue full", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Request queue implementation

func newRequestQueue(maxSize int) *requestQueue {
	q := &requestQueue{
		items:   list.New(),
		maxSize: maxSize,
	}
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

func (q *requestQueue) waitForSlot(ctx context.Context, timeout time.Duration) (bool, error) {
	q.mu.Lock()

	// Check if queue is full
	if q.items.Len() >= q.maxSize {
		q.mu.Unlock()
		return false, nil
	}

	// Create result channel
	resultCh := make(chan bool, 1)
	req := &queuedRequest{
		addedAt:  time.Now(),
		resultCh: resultCh,
	}
	q.items.PushBack(req)
	q.mu.Unlock()

	// Wait for result or timeout
	select {
	case result := <-resultCh:
		return result, nil
	case <-time.After(timeout):
		q.removeRequest(req)
		return false, nil
	case <-ctx.Done():
		q.removeRequest(req)
		return false, ctx.Err()
	}
}

func (q *requestQueue) releaseSlots(count int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	released := 0
	for released < count && q.items.Len() > 0 {
		front := q.items.Front()
		if front == nil {
			break
		}

		req := front.Value.(*queuedRequest)
		q.items.Remove(front)

		select {
		case req.resultCh <- true:
			released++
		default:
			// Channel closed or full, skip
		}
	}
}

func (q *requestQueue) removeRequest(req *queuedRequest) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for e := q.items.Front(); e != nil; e = e.Next() {
		if e.Value.(*queuedRequest) == req {
			q.items.Remove(e)
			break
		}
	}
}

func (q *requestQueue) length() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.items.Len()
}

// Errors
var (
	ErrQueueFull    = errors.New("rate limit queue is full")
	ErrQueueTimeout = errors.New("rate limit queue timeout")
)
