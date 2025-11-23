# DuckDB Integration Guide

This guide covers the DuckDB integration for the NDC ClickHouse connector, enabling hybrid analytics with server-side query caching and client-side browser analytics.

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Server-Side DuckDB Cache](#server-side-duckdb-cache)
4. [Hybrid Query Engine](#hybrid-query-engine)
5. [Client-Side DuckDB-WASM](#client-side-duckdb-wasm)
6. [Configuration](#configuration)
7. [API Reference](#api-reference)
8. [Best Practices](#best-practices)

## Overview

The DuckDB integration provides three layers of analytics optimization:

| Layer | Technology | Use Case |
|-------|------------|----------|
| **Server Cache** | DuckDB (Go) | Cache ClickHouse results for repeated queries |
| **Hybrid Engine** | Query Router | Route queries to optimal engine based on cost |
| **Client Analytics** | DuckDB-WASM | Browser-based analytics on cached data |

### Benefits

- **Reduced Latency**: Local queries on cached data
- **Lower Costs**: Fewer ClickHouse queries
- **Offline Support**: Analytics without network connectivity
- **Better UX**: Instant dashboard updates

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Client (Browser)                             │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    DuckDB-WASM                               │    │
│  │  • Local SQL queries                                         │    │
│  │  • Cached data from server                                   │    │
│  │  • Instant aggregations                                      │    │
│  └─────────────────────────────────────────────────────────────┘    │
└────────────────────────────┬────────────────────────────────────────┘
                             │ Export/Sync
┌────────────────────────────▼────────────────────────────────────────┐
│                        NDC Connector                                 │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                   Hybrid Query Router                        │    │
│  │  • Cost-based routing                                        │    │
│  │  • Freshness checking                                        │    │
│  │  • Fallback handling                                         │    │
│  └───────────────┬────────────────────┬────────────────────────┘    │
│                  │                    │                              │
│  ┌───────────────▼────────┐  ┌───────▼────────────────────────┐    │
│  │    DuckDB Cache        │  │       ClickHouse               │    │
│  │  • Parquet storage     │  │  • Real-time data              │    │
│  │  • LRU eviction        │  │  • Distributed queries         │    │
│  │  • TTL management      │  │  • Full dataset                │    │
│  └────────────────────────┘  └────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

## Server-Side DuckDB Cache

### Installation

```go
import "github.com/your-org/ndc-clickhouse-go/duckdb"
```

### Basic Usage

```go
// Create DuckDB client
cfg := &duckdb.Config{
    Enabled:        true,
    DatabasePath:   ":memory:",  // or "/path/to/cache.db"
    MaxCacheSizeMB: 1024,
    DefaultMaxAge:  time.Hour,
}

client, err := duckdb.NewClient(cfg)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Cache data from ClickHouse
data := []map[string]interface{}{
    {"id": 1, "name": "Alice", "age": 30},
    {"id": 2, "name": "Bob", "age": 25},
}

err = client.CacheData(ctx, "users", data)
if err != nil {
    log.Fatal(err)
}

// Query cached data
results, err := client.Query(ctx, "SELECT * FROM users WHERE age > 25")
```

### Parquet Export/Import

```go
// Export to Parquet
err = client.ExportToParquet(ctx, "users", "/tmp/users.parquet")

// Import from Parquet
err = client.ImportFromParquet(ctx, "users", "/tmp/users.parquet")
```

### Cache Statistics

```go
stats, err := client.GetCacheStats()
fmt.Printf("Total rows: %d\n", stats.TotalRows)
fmt.Printf("Collections: %d\n", stats.TotalCollections)

for name, coll := range stats.Collections {
    fmt.Printf("  %s: %d rows, fresh: %v\n", name, coll.RowCount, coll.IsFresh)
}
```

## Hybrid Query Engine

The hybrid engine routes queries to the optimal execution engine.

### Configuration

```go
hybridConfig := &duckdb.HybridConfig{
    Enabled:               true,
    PreferDuckDBUnderRows: 100000,  // Use DuckDB for small tables
    CostThreshold:         0.5,     // Use DuckDB if cost < 50% of ClickHouse
    EnableQueryAnalysis:   true,
    FallbackOnError:       true,
}
```

### Query Routing

```go
router := duckdb.NewQueryRouter(duckdbClient, hybridConfig)

query := &duckdb.QueryPlan{
    Collection: "users",
    Filters:    map[string]interface{}{"department": "Engineering"},
    Limit:      100,
}

decision, err := router.RouteQuery(ctx, query)
fmt.Printf("Engine: %s, Reason: %s\n", decision.Engine, decision.Reason)
```

### Execute with Routing

```go
result, err := router.Execute(ctx, query, clickhouseClient)
fmt.Printf("Executed on: %s\n", result.Decision.Engine)
fmt.Printf("Duration: %v\n", result.Duration)
```

### Routing Statistics

```go
stats := router.GetStats()
fmt.Printf("Total queries: %d\n", stats.TotalQueries)
fmt.Printf("ClickHouse: %d\n", stats.ClickHouseQueries)
fmt.Printf("DuckDB: %d\n", stats.DuckDBQueries)
fmt.Printf("Fallbacks: %d\n", stats.Fallbacks)
```

## Client-Side DuckDB-WASM

### Installation

```bash
cd nextjs-clickhouse-app
npm install @duckdb/duckdb-wasm apache-arrow
```

### React Hooks

#### Initialize DuckDB

```tsx
import { useDuckDB } from '@/lib/duckdb';

function App() {
  const { initialized, loading, error } = useDuckDB();

  if (loading) return <div>Initializing DuckDB...</div>;
  if (error) return <div>Error: {error.message}</div>;
  if (!initialized) return <div>DuckDB not ready</div>;

  return <Analytics />;
}
```

#### Load Data

```tsx
import { useDuckDBLoader } from '@/lib/duckdb';

function DataLoader() {
  const { loadData, loadFromParquet, loading, error } = useDuckDBLoader();

  const handleLoadFromServer = async () => {
    const response = await fetch('/api/users');
    const data = await response.json();
    await loadData('users', data);
  };

  const handleLoadParquet = async () => {
    await loadFromParquet('users', '/data/users.parquet');
  };

  return (
    <div>
      <button onClick={handleLoadFromServer} disabled={loading}>
        Load from Server
      </button>
      <button onClick={handleLoadParquet} disabled={loading}>
        Load Parquet
      </button>
    </div>
  );
}
```

#### Execute Queries

```tsx
import { useDuckDBQuery } from '@/lib/duckdb';

function QueryResults() {
  const { data, loading, error, refetch } = useDuckDBQuery(
    'SELECT department, COUNT(*) as count FROM users GROUP BY department'
  );

  if (loading) return <div>Loading...</div>;
  if (error) return <div>Error: {error.message}</div>;

  return (
    <div>
      <p>{data?.rowCount} rows in {data?.executionTimeMs.toFixed(2)}ms</p>
      <table>
        <thead>
          <tr>
            {data?.columns.map(col => <th key={col}>{col}</th>)}
          </tr>
        </thead>
        <tbody>
          {data?.rows.map((row, i) => (
            <tr key={i}>
              {data.columns.map(col => <td key={col}>{row[col]}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

#### Aggregations

```tsx
import { useDuckDBAggregation } from '@/lib/duckdb';

function SalesStats() {
  const { data } = useDuckDBAggregation('orders', {
    groupBy: ['region'],
    aggregations: [
      { column: 'amount', function: 'sum', alias: 'total_sales' },
      { column: 'amount', function: 'avg', alias: 'avg_order' },
      { column: '*', function: 'count', alias: 'order_count' },
    ],
    orderBy: 'total_sales DESC',
    limit: 10,
  });

  return (
    <div>
      {data?.rows.map(row => (
        <div key={row.region}>
          {row.region}: ${row.total_sales} ({row.order_count} orders)
        </div>
      ))}
    </div>
  );
}
```

#### Sync with Server

```tsx
import { useDuckDBSync } from '@/lib/duckdb';

function DataSync() {
  const { sync, syncing, lastSync, error } = useDuckDBSync(
    async () => {
      const response = await fetch('/api/analytics/export');
      return response.json();
    },
    {
      autoSync: true,
      syncInterval: 60000, // 1 minute
    }
  );

  return (
    <div>
      <button onClick={sync} disabled={syncing}>
        {syncing ? 'Syncing...' : 'Sync Now'}
      </button>
      {lastSync && <p>Last sync: {lastSync.toLocaleString()}</p>}
    </div>
  );
}
```

## Configuration

### Server Configuration (configuration.json)

```json
{
  "duckdb": {
    "enabled": true,
    "database_path": ":memory:",
    "max_cache_size_mb": 1024,
    "default_max_age": "1h",
    "export_format": "parquet",
    "parquet_compression": "snappy",
    "collections": {
      "users": {
        "enabled": true,
        "max_age": "30m",
        "max_rows": 100000,
        "refresh_interval": "15m"
      },
      "events": {
        "enabled": true,
        "max_age": "5m",
        "max_rows": 1000000
      }
    },
    "hybrid": {
      "enabled": true,
      "prefer_duckdb_under_rows": 100000,
      "cost_threshold": 0.5,
      "enable_query_analysis": true,
      "fallback_on_error": true
    }
  }
}
```

### Client Configuration

```typescript
import { getDuckDBClient } from '@/lib/duckdb';

const client = getDuckDBClient({
  maxMemoryMB: 512,
  enableLogging: process.env.NODE_ENV === 'development',
});
```

## API Reference

### Server-Side (Go)

#### DuckDB Client

| Method | Description |
|--------|-------------|
| `NewClient(cfg)` | Create new DuckDB client |
| `CacheData(ctx, collection, data)` | Cache data in DuckDB |
| `Query(ctx, sql, args...)` | Execute SQL query |
| `QueryCollection(ctx, collection, filters, limit)` | Query with filters |
| `IsCached(collection)` | Check if collection is cached and fresh |
| `InvalidateCollection(ctx, collection)` | Remove cached collection |
| `Clear(ctx)` | Clear all cached data |
| `ExportToParquet(ctx, collection, path)` | Export to Parquet |
| `ImportFromParquet(ctx, collection, path)` | Import from Parquet |
| `GetCacheStats()` | Get cache statistics |
| `Close()` | Close connection |

#### Hybrid Router

| Method | Description |
|--------|-------------|
| `NewQueryRouter(client, config)` | Create query router |
| `RouteQuery(ctx, query)` | Get routing decision |
| `Execute(ctx, query, executor)` | Execute with routing |
| `GetStats()` | Get routing statistics |
| `ResetStats()` | Reset statistics |

### Client-Side (TypeScript)

#### Hooks

| Hook | Description |
|------|-------------|
| `useDuckDB()` | Initialize DuckDB-WASM |
| `useDuckDBQuery(sql, options)` | Execute SQL query |
| `useDuckDBLoader()` | Load data into DuckDB |
| `useDuckDBStats()` | Get cache statistics |
| `useDuckDBAggregation(table, options)` | Run aggregation queries |
| `useDuckDBTables()` | Manage cached tables |
| `useDuckDBSync(fetcher, options)` | Sync with server |
| `useDuckDBComparison(local, remote, options)` | Compare local vs remote |

## Best Practices

### 1. Cache Strategy

```go
// Cache frequently accessed, slowly changing data
cfg.Collections["users"] = duckdb.CollectionConfig{
    Enabled:         true,
    MaxAge:          time.Hour,     // Refresh hourly
    RefreshInterval: time.Minute * 30,
}

// Cache event data with shorter TTL
cfg.Collections["recent_events"] = duckdb.CollectionConfig{
    Enabled:  true,
    MaxAge:   time.Minute * 5,  // Short TTL for freshness
    MaxRows:  100000,           // Limit size
}
```

### 2. Memory Management

```typescript
// Client-side: Monitor memory usage
const { stats } = useDuckDBStats();
if (stats.totalSizeBytes > 100 * 1024 * 1024) {  // 100MB
  // Clear old data
  await client.clearAll();
}
```

### 3. Error Handling

```go
// Server: Enable fallback on DuckDB errors
hybridConfig.FallbackOnError = true

// Client: Handle initialization failures
const { error } = useDuckDB();
if (error) {
  // Fall back to server-side queries
}
```

### 4. Performance Monitoring

```go
// Track routing decisions
stats := router.GetStats()
duckdbRatio := float64(stats.DuckDBQueries) / float64(stats.TotalQueries)
if duckdbRatio < 0.5 {
    // Consider caching more data
}
```

### 5. Data Freshness

```tsx
// Show data age to users
function DataStatus({ tableName }) {
  const { tables } = useDuckDBTables();
  const table = tables.find(t => t.name === tableName);

  const age = Date.now() - table.lastUpdated.getTime();
  const isStale = age > 5 * 60 * 1000; // 5 minutes

  return (
    <span className={isStale ? 'text-yellow-500' : 'text-green-500'}>
      Updated {formatDistanceToNow(table.lastUpdated)} ago
    </span>
  );
}
```

## Troubleshooting

### DuckDB-WASM Not Loading

```typescript
// Check browser compatibility
if (typeof WebAssembly === 'undefined') {
  console.error('WebAssembly not supported');
}

// Check for CORS issues with CDN bundles
// Use local bundles if needed
```

### Memory Issues

```sql
-- Check DuckDB memory usage
PRAGMA database_size;
PRAGMA memory_limit;
```

### Query Performance

```sql
-- Explain query plan
EXPLAIN ANALYZE SELECT * FROM users WHERE age > 25;
```
