package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	limits   map[string]*RateLimit
	buckets  map[string]*tokenBucket
	mu       sync.RWMutex
	config   *RateLimitConfig
}

// RateLimit defines rate limiting rules
type RateLimit struct {
	// Requests per window
	Limit int `json:"limit"`

	// Time window
	Window time.Duration `json:"window"`

	// Burst size (allows temporary bursts above limit)
	Burst int `json:"burst"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// Enable rate limiting
	Enabled bool `json:"enabled"`

	// Default limit for all requests
	Default *RateLimit `json:"default"`

	// Per-role limits
	RoleLimits map[string]*RateLimit `json:"role_limits"`

	// Per-endpoint limits
	EndpointLimits map[string]*RateLimit `json:"endpoint_limits"`

	// Per-IP limits
	IPLimits map[string]*RateLimit `json:"ip_limits"`
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Enabled: true,
		Default: &RateLimit{
			Limit:  100,
			Window: time.Minute,
			Burst:  10,
		},
		RoleLimits: map[string]*RateLimit{
			"admin": {Limit: 1000, Window: time.Minute, Burst: 100},
			"user":  {Limit: 100, Window: time.Minute, Burst: 20},
			"anonymous": {Limit: 20, Window: time.Minute, Burst: 5},
		},
	}
}

// tokenBucket implements the token bucket algorithm
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	return &RateLimiter{
		limits:  make(map[string]*RateLimit),
		buckets: make(map[string]*tokenBucket),
		config:  config,
	}
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow(key string, limit *RateLimit) bool {
	if !rl.config.Enabled {
		return true
	}

	if limit == nil {
		limit = rl.config.Default
	}

	rl.mu.Lock()
	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = &tokenBucket{
			tokens:     float64(limit.Burst),
			maxTokens:  float64(limit.Burst),
			refillRate: float64(limit.Limit) / limit.Window.Seconds(),
			lastRefill: time.Now(),
		}
		rl.buckets[key] = bucket
	}
	rl.mu.Unlock()

	return bucket.take()
}

// AllowByRole checks if a request is allowed for a specific role
func (rl *RateLimiter) AllowByRole(role, identifier string) bool {
	if !rl.config.Enabled {
		return true
	}

	limit := rl.config.Default
	if roleLimit, ok := rl.config.RoleLimits[role]; ok {
		limit = roleLimit
	}

	key := fmt.Sprintf("role:%s:%s", role, identifier)
	return rl.Allow(key, limit)
}

// AllowByIP checks if a request is allowed for a specific IP
func (rl *RateLimiter) AllowByIP(ip string) bool {
	if !rl.config.Enabled {
		return true
	}

	limit := rl.config.Default
	if ipLimit, ok := rl.config.IPLimits[ip]; ok {
		limit = ipLimit
	}

	key := fmt.Sprintf("ip:%s", ip)
	return rl.Allow(key, limit)
}

// AllowByEndpoint checks if a request is allowed for a specific endpoint
func (rl *RateLimiter) AllowByEndpoint(endpoint, identifier string) bool {
	if !rl.config.Enabled {
		return true
	}

	limit := rl.config.Default
	if endpointLimit, ok := rl.config.EndpointLimits[endpoint]; ok {
		limit = endpointLimit
	}

	key := fmt.Sprintf("endpoint:%s:%s", endpoint, identifier)
	return rl.Allow(key, limit)
}

// take removes a token from the bucket
func (b *tokenBucket) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	// Check if we have tokens
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// GetRemainingTokens returns the remaining tokens for a key
func (rl *RateLimiter) GetRemainingTokens(key string) int {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if !exists {
		return 0
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	return int(bucket.tokens)
}

// Reset resets the rate limiter for a specific key
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	delete(rl.buckets, key)
	rl.mu.Unlock()
}

// ResetAll resets all rate limiters
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	rl.buckets = make(map[string]*tokenBucket)
	rl.mu.Unlock()
}

// Cleanup removes expired buckets
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, bucket := range rl.buckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastRefill) > maxAge {
			delete(rl.buckets, key)
		}
		bucket.mu.Unlock()
	}
}

// RateLimitMiddleware creates an HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get identifier (IP or user ID)
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
		if !rl.AllowByRole(role, identifier) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitContext provides rate limit information in context
type RateLimitContext struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
}

// WithRateLimitContext adds rate limit info to context
func WithRateLimitContext(ctx context.Context, info *RateLimitContext) context.Context {
	return context.WithValue(ctx, rateLimitContextKey{}, info)
}

// GetRateLimitContext retrieves rate limit info from context
func GetRateLimitContext(ctx context.Context) *RateLimitContext {
	if v := ctx.Value(rateLimitContextKey{}); v != nil {
		return v.(*RateLimitContext)
	}
	return nil
}

type rateLimitContextKey struct{}
