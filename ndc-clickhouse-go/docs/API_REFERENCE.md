# API Reference

Complete API reference for the NDC ClickHouse connector.

## Table of Contents

1. [NDC Endpoints](#ndc-endpoints)
2. [Console API](#console-api)
3. [Configuration API](#configuration-api)
4. [Package APIs](#package-apis)

## NDC Endpoints

The connector implements the [Hasura NDC Specification](https://hasura.io/docs/3.0/connectors/ndc-spec/).

### GET /capabilities

Returns connector capabilities.

**Response:**
```json
{
  "version": "0.1.0",
  "capabilities": {
    "query": {
      "aggregates": {},
      "variables": {}
    },
    "mutation": {
      "transactional": {},
      "explain": {}
    },
    "relationships": {}
  }
}
```

### GET /schema

Returns the GraphQL schema derived from ClickHouse.

**Response:**
```json
{
  "scalar_types": {
    "Int": {
      "aggregate_functions": {
        "sum": { "result_type": "Int" },
        "avg": { "result_type": "Float" },
        "min": { "result_type": "Int" },
        "max": { "result_type": "Int" },
        "count": { "result_type": "Int" }
      },
      "comparison_operators": {
        "_eq": { "type": "equal" },
        "_neq": { "type": "custom", "argument_type": "Int" },
        "_gt": { "type": "custom", "argument_type": "Int" },
        "_gte": { "type": "custom", "argument_type": "Int" },
        "_lt": { "type": "custom", "argument_type": "Int" },
        "_lte": { "type": "custom", "argument_type": "Int" }
      }
    }
  },
  "object_types": {
    "users": {
      "fields": {
        "id": { "type": { "type": "named", "name": "Int" } },
        "name": { "type": { "type": "named", "name": "String" } }
      }
    }
  },
  "collections": [
    {
      "name": "users",
      "type": "users",
      "arguments": {}
    }
  ],
  "functions": [],
  "procedures": []
}
```

### POST /query

Executes a query against ClickHouse.

**Request:**
```json
{
  "collection": "users",
  "query": {
    "fields": {
      "id": { "type": "column", "column": "id" },
      "name": { "type": "column", "column": "name" }
    },
    "where": {
      "type": "binary_comparison_operator",
      "column": { "type": "column", "name": "id" },
      "operator": "_eq",
      "value": { "type": "scalar", "value": 1 }
    },
    "order_by": {
      "elements": [
        { "target": { "type": "column", "name": "id" }, "order_direction": "asc" }
      ]
    },
    "limit": 10,
    "offset": 0
  },
  "arguments": {},
  "collection_relationships": {}
}
```

**Response:**
```json
[
  {
    "rows": [
      { "id": 1, "name": "Alice" }
    ]
  }
]
```

### POST /mutation

Executes a mutation (INSERT).

**Request:**
```json
{
  "operations": [
    {
      "type": "procedure",
      "name": "insert_users",
      "arguments": {
        "objects": [
          { "id": 1, "name": "Alice" },
          { "id": 2, "name": "Bob" }
        ]
      }
    }
  ]
}
```

**Response:**
```json
{
  "operation_results": [
    {
      "affected_rows": 2
    }
  ]
}
```

### POST /explain

Returns the generated SQL for a query.

**Request:** Same as `/query`

**Response:**
```json
{
  "details": {
    "sql": "SELECT id, name FROM users WHERE id = ? ORDER BY id ASC LIMIT 10",
    "parameters": [1]
  }
}
```

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "healthy"
}
```

## Console API

The web console provides additional REST endpoints.

### GET /api/schema

Returns database schema information.

**Response:**
```json
{
  "tables": [
    {
      "name": "users",
      "database": "default",
      "engine": "MergeTree"
    }
  ],
  "columns": {
    "users": [
      {
        "name": "id",
        "type": "UInt64",
        "nullable": false
      }
    ]
  }
}
```

### GET /api/tables

Returns list of tables.

**Response:**
```json
[
  {
    "name": "users",
    "database": "default",
    "engine": "MergeTree",
    "total_rows": 1000000
  }
]
```

### POST /api/query

Executes raw SQL (admin only).

**Request:**
```json
{
  "sql": "SELECT * FROM users LIMIT 10"
}
```

**Response:**
```json
{
  "columns": ["id", "name"],
  "rows": [
    { "id": 1, "name": "Alice" }
  ],
  "rowCount": 1,
  "duration_ms": 15
}
```

### GET /api/stats

Returns query statistics.

**Response:**
```json
{
  "total_queries": 1000,
  "total_errors": 5,
  "average_duration": "15ms",
  "slow_queries": 10,
  "cache_hits": 800,
  "cache_misses": 200,
  "queries_per_minute": 100.5
}
```

### GET /api/health

Detailed health check.

**Response:**
```json
{
  "status": "healthy",
  "database": "default",
  "connection": "ok",
  "latency_ms": 5
}
```

## Configuration API

### GET /api/config

Returns current configuration.

**Response:**
```json
{
  "connection": {
    "url": "clickhouse://localhost:9000",
    "database": "default"
  },
  "tables": {},
  "relationships": {},
  "permissions": {}
}
```

### PUT /api/config

Updates configuration (requires restart).

**Request:**
```json
{
  "connection": {
    "url": "clickhouse://localhost:9000",
    "database": "analytics"
  }
}
```

## Package APIs

### Cache Package

```go
import "github.com/your-org/ndc-clickhouse-go/cache"

// Create basic LRU cache
c := cache.NewInMemoryCache(1000)

// Basic operations
c.Set("key", value, 5*time.Minute)
value, found := c.Get("key")
c.Delete("key")
c.Clear()

// Statistics
stats := c.Stats()
// stats.Hits, stats.Misses, stats.Size, stats.HitRate

// Query-specific cache (simple)
qc := cache.NewQueryCache(&cache.QueryCacheConfig{
    Enabled:    true,
    MaxSize:    1000,
    DefaultTTL: 5 * time.Minute,
})

// Cache query results
qc.Set("users", queryParams, results)
results, found := qc.Get("users", queryParams)

// Cache wrapper
result, err := qc.WithCache("users", queryParams, func() (interface{}, error) {
    return db.Query(sql)
})
```

#### Advanced Cache (Configurable)

```go
import "github.com/your-org/ndc-clickhouse-go/cache"

// Create advanced cache with per-collection settings
cfg := &cache.AdvancedCacheConfig{
    Enabled:              true,
    MaxSize:              10000,
    DefaultTTL:           5 * time.Minute,
    CleanupInterval:      time.Minute,
    StatsEnabled:         true,
    InvalidateOnMutation: true,
    Collections: map[string]cache.CollectionCacheSettings{
        "users": {
            Enabled: true,
            TTL:     10 * time.Minute,
            MaxSize: 1000,
        },
        "events": {
            Enabled: true,
            TTL:     time.Minute,
        },
    },
}

ac := cache.NewAdvancedCache(cfg)
defer ac.Close() // Stops cleanup goroutine

// Per-collection caching
ac.Set("users", "key1", value)
value, found := ac.Get("users", "key1")
ac.Delete("users", "key1")

// Query caching with automatic key generation
ac.SetQuery("users", queryParams, results)
results, found := ac.GetQuery("users", queryParams)

// Cache wrapper for queries
result, err := ac.WithQuery("users", queryParams, func() (interface{}, error) {
    return db.Query(sql)
})

// Collection invalidation
ac.InvalidateCollection("users")
ac.Clear() // Clear all caches

// Advanced statistics
stats := ac.Stats()
// stats.TotalHits, stats.TotalMisses, stats.HitRate
// stats.MainCache - main cache stats
// stats.Collections - per-collection stats

// Check if cache should invalidate on mutations
if ac.ShouldInvalidateOnMutation() {
    ac.InvalidateCollection("users")
}
```

#### Cache Statistics Structure

```go
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
```

#### Creating Config from JSON

```go
// Parse cache config from configuration map
m := map[string]interface{}{
    "enabled":          true,
    "max_size":         float64(10000),
    "default_ttl":      "5m",
    "cleanup_interval": "1m",
    "collections": map[string]interface{}{
        "users": map[string]interface{}{
            "enabled":  true,
            "ttl":      "10m",
            "max_size": float64(1000),
        },
    },
}

cfg, err := cache.ConfigFromMap(m)
ac := cache.NewAdvancedCache(cfg)
```

### Analytics Package

```go
import "github.com/your-org/ndc-clickhouse-go/analytics"

// Create logger
logger, _ := analytics.NewQueryLogger(&analytics.LoggerConfig{
    Enabled:       true,
    MaxInMemory:   10000,
    SlowThreshold: time.Second,
})

// Log queries
logger.Log(analytics.QueryLog{
    Collection: "users",
    Operation:  "query",
    SQL:        "SELECT * FROM users",
    Duration:   50 * time.Millisecond,
    RowCount:   100,
})

// Get statistics
stats := logger.GetStats()
// stats.TotalQueries, stats.TotalErrors, stats.SlowQueries

// Get recent queries
recent := logger.GetRecentQueries(10)
errors := logger.GetErrorQueries(10)
slow := logger.GetSlowQueries(10)

// Context-based logging
ctx := logger.StartQuery(context.Background(), "users", "query")
ctx.WithRole("admin").WithUserID("user-123")
// ... execute query ...
ctx.End(rowCount, err)
```

### Middleware Package

#### Authentication

```go
import "github.com/your-org/ndc-clickhouse-go/middleware"

// Create authenticator
auth := middleware.NewAuthenticator(&middleware.AuthConfig{
    Enabled: true,
    APIKeys: []middleware.APIKey{
        {
            Key:         "secret-key",
            Name:        "Admin Key",
            Roles:       []string{"admin"},
            DefaultRole: "admin",
        },
    },
    AllowAnonymous: true,
    AnonymousRole:  "guest",
})

// Use as middleware
handler := auth.Middleware(yourHandler)

// Get auth info in handler
func handler(w http.ResponseWriter, r *http.Request) {
    info := middleware.GetAuthInfo(r.Context())
    if info != nil {
        userID := info.UserID
        role := info.Role
        vars := info.GetSessionVariables()
    }
}
```

#### Rate Limiting

```go
import "github.com/your-org/ndc-clickhouse-go/middleware"

// Create rate limiter
limiter := middleware.NewRateLimiter(&middleware.RateLimitConfig{
    Enabled: true,
    Default: &middleware.RateLimit{
        Limit:  100,
        Window: time.Minute,
        Burst:  10,
    },
    RoleLimits: map[string]*middleware.RateLimit{
        "admin": {Limit: 1000, Window: time.Minute, Burst: 100},
        "user":  {Limit: 100, Window: time.Minute, Burst: 20},
    },
})

// Use as middleware
handler := limiter.Middleware(yourHandler)

// Manual checks
allowed := limiter.Allow("key", nil)
allowed := limiter.AllowByRole("admin", "user-123")
allowed := limiter.AllowByIP("192.168.1.1")

// Cleanup old buckets
limiter.Cleanup(10 * time.Minute)
```

### Subscription Package

```go
import "github.com/your-org/ndc-clickhouse-go/subscription"

// Create manager
manager := subscription.NewManager(clickhouseClient, 5*time.Second)
manager.Start()
defer manager.Stop()

// Subscribe to query
id, ch := manager.Subscribe(subscription.SubscriptionRequest{
    Collection: "users",
    Query:      queryParams,
    Variables:  sessionVars,
})

// Receive updates
go func() {
    for result := range ch {
        // Handle new data
        fmt.Println(result.Data)
    }
}()

// Unsubscribe
manager.Unsubscribe(id)
```

### ClickHouse Multi-Client

```go
import "github.com/your-org/ndc-clickhouse-go/clickhouse"

// Create multi-client
multi := clickhouse.NewMultiClient()

// Add databases
multi.AddDatabase("analytics", analyticsClient)
multi.AddDatabase("events", eventsClient)

// Get specific client
client := multi.GetClient("analytics")

// Query across all databases
results, err := multi.QueryAll(ctx, "SELECT count(*) FROM events")
for db, rows := range results {
    fmt.Printf("%s: %v\n", db, rows)
}
```

### Telemetry Package

```go
import "github.com/your-org/ndc-clickhouse-go/telemetry"

// Initialize telemetry from config
cfg := telemetry.DefaultConfig()
cfg.Enabled = true
cfg.ServiceName = "my-connector"
cfg.Tracing.Enabled = true
cfg.Tracing.Exporter = "otlp"
cfg.Tracing.Endpoint = "localhost:4317"
cfg.Metrics.Enabled = true
cfg.Metrics.Exporter = "prometheus"

tel, err := telemetry.New(ctx, cfg)
if err != nil {
    log.Fatal(err)
}
defer tel.Shutdown(ctx)

// Or initialize from file
tel, err := telemetry.NewFromFile(ctx, "telemetry.json")

// Use HTTP middleware
handler := telemetry.HTTPMiddleware(nil)(yourHandler)

// Custom middleware config
middlewareCfg := &telemetry.HTTPMiddlewareConfig{
    ServiceName:  "my-service",
    SkipPaths:    []string{"/health", "/metrics"},
    UserIDHeader: "X-Hasura-User-Id",
    RoleHeader:   "X-Hasura-Role",
}
handler := telemetry.HTTPMiddleware(middlewareCfg)(yourHandler)

// Manual span creation
ctx, span := telemetry.StartSpanFromContext(ctx, "operation-name")
defer span.End()

// Add attributes to current span
telemetry.SetAttributes(ctx,
    attribute.String("user.id", userID),
    attribute.String("collection", "users"),
)

// Record errors
if err != nil {
    telemetry.RecordError(ctx, err)
}

// Database query tracing
dbTracer := telemetry.NewClickHouseQueryTracer("mydb", true)
ctx, finish := dbTracer.TraceSelect(ctx, "users", "SELECT * FROM users")
// ... execute query ...
finish(rowCount, err)

// Convenience function for query tracing
ctx, finish := telemetry.TraceSelectQuery(ctx, "users", sql)
rows, err := client.Query(ctx, sql)
finish(int64(len(rows)), err)

// Record metrics manually
metrics := telemetry.GetMetrics()
metrics.RecordQuery(ctx, "users", "SELECT", duration, rowCount, nil)
metrics.RecordCacheHit(ctx, "query")
metrics.RecordCacheMiss(ctx, "query")
metrics.RecordRateLimit(ctx, "token_bucket", "anonymous")
metrics.RecordHTTPRequest(ctx, "GET", "/api/query", 200, duration)

// Prometheus endpoint
http.Handle("/metrics", tel.Metrics().PrometheusHandler())

// Get trace context for logging
traceID := telemetry.WithTraceID(ctx)
spanID := telemetry.WithSpanID(ctx)
log.Printf("trace_id=%s span_id=%s query completed", traceID, spanID)

// Traced HTTP client
client := telemetry.NewTraceHTTPClient(nil, "my-client")
resp, err := client.Get(ctx, "https://api.example.com/data")
```

#### Telemetry Configuration Types

```go
// Main configuration
type Config struct {
    Enabled            bool
    ServiceName        string
    ServiceVersion     string
    Environment        string
    ResourceAttributes map[string]string
    Tracing           TracingConfig
    Metrics           MetricsConfig
}

// Tracing configuration
type TracingConfig struct {
    Enabled         bool
    Exporter        string  // "otlp", "jaeger", "zipkin", "stdout", "none"
    Endpoint        string
    Protocol        string  // "grpc" or "http"
    SamplingStrategy string // "always_on", "always_off", "trace_id_ratio", "parent_based"
    SamplingRatio   float64
    TraceDB         bool
    TraceHTTP       bool
    TraceGraphQL    bool
}

// Metrics configuration
type MetricsConfig struct {
    Enabled              bool
    Exporter             string // "otlp", "prometheus", "stdout", "none"
    Endpoint             string
    PrometheusPort       int
    PrometheusPath       string
    MetricPrefix         string
    CollectDBMetrics     bool
    CollectHTTPMetrics   bool
    CollectRuntimeMetrics bool
}
```

### Leaky Bucket Rate Limiter

```go
import "github.com/your-org/ndc-clickhouse-go/middleware"

// Create leaky bucket limiter
lb := middleware.NewLeakyBucket(&middleware.LeakyBucketConfig{
    Enabled:      true,
    BucketSize:   100,           // Max concurrent requests
    LeakRate:     10,            // Requests processed per second
    QueueSize:    50,            // Buffer for overflow
    QueueTimeout: 30 * time.Second,
    RoleConfigs: map[string]*middleware.LeakyBucketRoleConfig{
        "admin": {BucketSize: 1000, LeakRate: 100, QueueSize: 500},
        "user":  {BucketSize: 100, LeakRate: 10, QueueSize: 50},
    },
})

// Non-blocking check
allowed := lb.Allow("user-key")
allowed := lb.AllowByRole("admin", "user-123")
allowed := lb.AllowByEndpoint("/api/heavy", "user-123")

// Blocking check with queue
allowed, err := lb.AllowWithQueue(ctx, "user-key")

// HTTP middleware
handler := lb.Middleware(yourHandler)          // Reject when full
handler := lb.MiddlewareWithQueue(yourHandler) // Queue when full

// Get statistics
stats := lb.GetStats("user-key")
// stats.CurrentLevel, stats.BucketSize, stats.QueueLength
// stats.TotalRequests, stats.DroppedRequests, stats.QueuedRequests
```

## Error Codes

| Code | Description |
|------|-------------|
| 400  | Bad Request - Invalid query or parameters |
| 401  | Unauthorized - Authentication required |
| 403  | Forbidden - Insufficient permissions |
| 404  | Not Found - Collection or field not found |
| 429  | Too Many Requests - Rate limit exceeded |
| 500  | Internal Server Error - Database or system error |

## Headers

### Request Headers

| Header | Description |
|--------|-------------|
| `Authorization` | Bearer token or API key |
| `X-API-Key` | API key authentication |
| `X-Hasura-Role` | User role for permissions |
| `X-Hasura-User-Id` | User ID for row-level security |
| `X-Hasura-Org-Id` | Organization ID |
| `X-Hasura-Tenant-Id` | Tenant ID for multi-tenancy |

### Response Headers

| Header | Description |
|--------|-------------|
| `X-Request-Id` | Unique request identifier |
| `X-RateLimit-Limit` | Rate limit ceiling |
| `X-RateLimit-Remaining` | Remaining requests |
| `Retry-After` | Seconds until rate limit resets (on 429) |
