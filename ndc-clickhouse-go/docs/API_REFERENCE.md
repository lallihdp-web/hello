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

// Create cache
cache := cache.NewInMemoryCache(1000)

// Basic operations
cache.Set("key", value, 5*time.Minute)
value, found := cache.Get("key")
cache.Delete("key")
cache.Clear()

// Statistics
stats := cache.Stats()
// stats.Hits, stats.Misses, stats.Size, stats.HitRate

// Query-specific cache
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
