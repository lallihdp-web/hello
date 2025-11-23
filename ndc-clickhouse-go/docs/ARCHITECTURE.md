# Architecture Overview

This document provides a comprehensive technical overview of the NDC ClickHouse connector architecture.

## Table of Contents

1. [System Overview](#system-overview)
2. [Component Architecture](#component-architecture)
3. [Data Flow](#data-flow)
4. [Package Structure](#package-structure)
5. [Key Design Decisions](#key-design-decisions)
6. [Performance Characteristics](#performance-characteristics)

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Hasura GraphQL Engine                          │
└─────────────────────────────────────────────────────────────────────────┘
                                     │
                                     │ NDC Protocol (HTTP/JSON)
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        NDC ClickHouse Connector                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ Auth         │  │ Rate Limit   │  │ Cache        │  │ Analytics    │ │
│  │ Middleware   │  │ Middleware   │  │ Layer        │  │ Logger       │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
│                                     │                                    │
│  ┌──────────────────────────────────┴────────────────────────────────┐  │
│  │                         Connector Core                             │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                │  │
│  │  │ Query       │  │ Mutation    │  │ Schema      │                │  │
│  │  │ Handler     │  │ Handler     │  │ Builder     │                │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                     │                                    │
│  ┌──────────────────────────────────┴────────────────────────────────┐  │
│  │                       ClickHouse Client                            │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                │  │
│  │  │ Connection  │  │ Query       │  │ Schema      │                │  │
│  │  │ Pool        │  │ Builder     │  │ Introspect  │                │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                                     │
                                     │ Native Protocol (TCP:9000)
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           ClickHouse Database                            │
└─────────────────────────────────────────────────────────────────────────┘
```

## Component Architecture

### 1. Connector Core (`connector/`)

The connector implements the Hasura NDC specification interface.

```go
type Connector struct{}

// Key interface methods:
func (c *Connector) ParseConfiguration(rawConfig string) (*Configuration, error)
func (c *Connector) TryInitState(ctx context.Context, config *Configuration) (*State, error)
func (c *Connector) GetSchema(ctx context.Context, config *Configuration, state *State) (*schema.SchemaResponse, error)
func (c *Connector) Query(ctx context.Context, config *Configuration, state *State, request *schema.QueryRequest) (*schema.QueryResponse, error)
func (c *Connector) Mutation(ctx context.Context, config *Configuration, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error)
```

**Files:**
- `connector.go` - Main connector implementation
- `query.go` - Query execution logic
- `mutation.go` - Mutation execution logic

### 2. ClickHouse Client (`clickhouse/`)

Manages database connections and introspection.

```go
type Client struct {
    conn     driver.Conn
    database string
}

// Key methods:
func (c *Client) GetTables(ctx context.Context) ([]TableInfo, error)
func (c *Client) GetColumns(ctx context.Context, table string) ([]ColumnInfo, error)
func (c *Client) Query(ctx context.Context, sql string, args ...interface{}) (driver.Rows, error)
func (c *Client) Exec(ctx context.Context, sql string, args ...interface{}) error
```

**Multi-Database Support (`multi.go`):**
```go
type MultiClient struct {
    clients map[string]*Client
}

func (mc *MultiClient) GetClient(name string) *Client
func (mc *MultiClient) QueryAll(ctx context.Context, sql string) (map[string][]map[string]interface{}, error)
```

### 3. Query Builder (`internal/query/`)

Translates NDC query requests to ClickHouse SQL.

```go
type Builder struct {
    collection string
    fields     map[string]schema.Field
    where      *schema.Expression
    orderBy    []schema.OrderByElement
    limit      *int
    offset     *int
}

func (b *Builder) Build() (*BuildResult, error)
```

**SQL Generation Flow:**
1. Parse NDC query request
2. Build SELECT clause from requested fields
3. Build WHERE clause from filter expressions
4. Build ORDER BY from ordering specification
5. Apply LIMIT/OFFSET for pagination
6. Return parameterized SQL with arguments

### 4. Schema Types (`schema/`)

Type mapping between ClickHouse and GraphQL.

```go
// Type mapping table:
// ClickHouse Type    → GraphQL Type
// Int8-Int256        → Int
// UInt8-UInt256      → Int (with validation)
// Float32/64         → Float
// String/FixedString → String
// UUID               → ID
// Date/DateTime      → DateTime (ISO8601)
// Array(T)           → [T]
// Nullable(T)        → T (nullable)
// Map(K,V)           → JSON
// Tuple(...)         → JSON
```

### 5. Configuration (`config/`)

Configuration management with environment variable expansion.

```go
type Configuration struct {
    Connection    ConnectionConfig     `json:"connection"`
    Tables        map[string]TableConfig `json:"tables"`
    Relationships RelationshipsConfig  `json:"relationships"`
    Permissions   PermissionsConfig    `json:"permissions"`
    NativeQueries map[string]NativeQuery `json:"native_queries"`
}
```

**Environment Variable Expansion:**
```json
{
  "connection": {
    "url": "${CLICKHOUSE_URL}",
    "database": "${CLICKHOUSE_DATABASE:-default}"
  }
}
```

### 6. Permissions (`config/permissions.go`)

Row-level security implementation.

```go
type PermissionChecker struct {
    config *PermissionsConfig
}

func (pc *PermissionChecker) CanSelect(role, table string) bool
func (pc *PermissionChecker) BuildFilterSQL(filter *FilterExpression, vars SessionVariables) string
```

**Permission Evaluation:**
1. Check if role is admin (bypass all checks)
2. Check table-level permissions
3. Apply row-level filters using session variables
4. Support inheritance from parent roles

### 7. Cache Layer (`cache/`)

LRU cache with TTL support.

```go
type InMemoryCache struct {
    maxSize   int
    items     map[string]*cacheItem
    lru       *list.List
}

// Performance characteristics:
// - Get: O(1) average
// - Set: O(1) average
// - Eviction: O(1) (LRU)
```

### 8. Analytics (`analytics/`)

Query logging and performance tracking.

```go
type QueryLogger struct {
    enabled       bool
    queries       []QueryLog
    stats         QueryStats
    slowThreshold time.Duration
}

// Tracked metrics:
// - Total queries/errors
// - Average duration
// - Per-collection stats
// - Per-operation stats
// - Slow query detection
```

### 9. Middleware (`middleware/`)

**Authentication (`auth.go`):**
- API key authentication
- JWT token validation
- Webhook authentication
- Header extraction for Hasura claims

**Rate Limiting (`ratelimit.go`):**
- Token bucket algorithm
- Per-role limits
- Per-IP limits
- Per-endpoint limits

## Data Flow

### Query Request Flow

```
1. HTTP Request arrives at connector
   ↓
2. Authentication middleware extracts user info
   ↓
3. Rate limiter checks request quota
   ↓
4. Query handler receives NDC request
   ↓
5. Permission checker validates access
   ↓
6. Cache lookup for identical query
   ↓ (miss)
7. Query builder generates SQL
   ↓
8. ClickHouse client executes query
   ↓
9. Results cached for future requests
   ↓
10. Analytics logger records query
   ↓
11. Response returned to Hasura
```

### Schema Introspection Flow

```
1. Connector starts with configuration
   ↓
2. Connect to ClickHouse
   ↓
3. Query system.tables for table list
   ↓
4. Query system.columns for each table
   ↓
5. Map ClickHouse types to GraphQL types
   ↓
6. Build NDC schema response
   ↓
7. Return to Hasura for GraphQL schema generation
```

## Package Structure

```
ndc-clickhouse-go/
├── cmd/
│   └── ndc-clickhouse/
│       ├── main.go          # Entry point
│       └── cli.go           # CLI commands
├── connector/
│   ├── connector.go         # NDC interface
│   ├── query.go             # Query handling
│   └── mutation.go          # Mutation handling
├── clickhouse/
│   ├── client.go            # Database client
│   └── multi.go             # Multi-database support
├── schema/
│   └── types.go             # Type mapping
├── internal/
│   └── query/
│       ├── builder.go       # SQL builder
│       └── executor.go      # Query executor
├── config/
│   ├── config.go            # Configuration
│   ├── relationships.go     # Relationship config
│   └── permissions.go       # Permission config
├── cache/
│   └── cache.go             # LRU cache
├── analytics/
│   └── logger.go            # Query analytics
├── middleware/
│   ├── auth.go              # Authentication
│   └── ratelimit.go         # Rate limiting
├── subscription/
│   └── manager.go           # Subscription support
├── console/
│   ├── server.go            # Web UI server
│   ├── static/              # CSS/JS assets
│   └── templates/           # HTML templates
└── docs/
    └── *.md                 # Documentation
```

## Key Design Decisions

### 1. No ORM - Direct SQL Generation

The connector generates SQL directly rather than using an ORM because:
- ClickHouse has unique SQL dialect features
- Direct SQL allows for optimization-specific queries
- Better control over query structure for OLAP workloads

### 2. Polling-Based Subscriptions

ClickHouse doesn't support native CDC (Change Data Capture), so subscriptions use polling:
- Configurable poll interval
- Result deduplication via hashing
- Suitable for dashboard-style updates

### 3. LRU Cache with TTL

The cache uses LRU eviction with TTL for:
- Memory-bounded caching
- Automatic stale data removal
- O(1) operations

### 4. Token Bucket Rate Limiting

Token bucket algorithm chosen for:
- Allows burst traffic
- Smooth rate limiting
- Per-key isolation

## Performance Characteristics

### Benchmark Results

```
Cache Operations:
  Set:        138 ns/op, 16 B/op
  Get:        110 ns/op, 0 B/op
  Concurrent: 296 ns/op, 8 B/op

Query Logger:
  Log:        551 ns/op, 784 B/op
  GetStats:   19 ns/op, 0 B/op

Rate Limiter:
  Allow:      119 ns/op, 0 B/op
  Concurrent: 300 ns/op, 4 B/op

Middleware:
  Auth:       707-1469 ns/op
  Full Stack: 1424 ns/op
```

### Scalability Considerations

1. **Connection Pooling**: ClickHouse driver handles connection pooling
2. **Concurrent Queries**: All components are thread-safe
3. **Memory Usage**: Cache size is bounded
4. **CPU Usage**: Efficient algorithms (O(1) for most operations)

### Recommended Tuning

| Setting | Development | Production |
|---------|-------------|------------|
| Cache Size | 100 | 10,000+ |
| Cache TTL | 1 min | 5-15 min |
| Rate Limit (user) | 100/min | 1000/min |
| Rate Limit (admin) | unlimited | 10000/min |
| Log Max Entries | 100 | 10,000 |
| Slow Query Threshold | 100ms | 1s |
