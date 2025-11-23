# Configuration Guide

This guide covers all configuration options for the NDC ClickHouse connector.

## Configuration File Structure

The configuration is stored in `configuration.json`:

```json
{
  "connection": { ... },
  "tables": { ... },
  "native_queries": { ... },
  "relationships": { ... },
  "permissions": { ... },
  "metadata": { ... }
}
```

## Connection Configuration

```json
{
  "connection": {
    "url": "clickhouse://localhost:9000",
    "database": "default",
    "username": "default",
    "password": "",
    "secure": false,
    "insecure_skip_verify": false,
    "max_open_conns": 10,
    "max_idle_conns": 5
  }
}
```

### Connection Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `url` | string | required | ClickHouse connection URL |
| `database` | string | required | Database name |
| `username` | string | `"default"` | Authentication username |
| `password` | string | `""` | Authentication password |
| `secure` | boolean | `false` | Enable TLS connection |
| `insecure_skip_verify` | boolean | `false` | Skip TLS certificate verification |
| `max_open_conns` | integer | `10` | Maximum open connections |
| `max_idle_conns` | integer | `5` | Maximum idle connections |

### URL Formats

```
# Native protocol (recommended for performance)
clickhouse://localhost:9000

# With authentication
clickhouse://user:password@localhost:9000

# Secure connection
clickhouses://localhost:9440

# HTTP protocol
http://localhost:8123
https://localhost:8443
```

### Environment Variables

Use `${VAR_NAME}` syntax for environment variable substitution:

```json
{
  "connection": {
    "url": "${CLICKHOUSE_URL}",
    "password": "${CLICKHOUSE_PASSWORD}"
  }
}
```

## Tables Configuration

Customize how tables are exposed in GraphQL:

```json
{
  "tables": {
    "user_accounts": {
      "alias": "Users",
      "exclude": false,
      "columns": {
        "password_hash": { "exclude": true },
        "email_address": { "alias": "email" }
      },
      "primary_key": ["id"]
    }
  }
}
```

### Table Options

| Option | Type | Description |
|--------|------|-------------|
| `alias` | string | GraphQL type name (defaults to table name) |
| `exclude` | boolean | Exclude table from schema |
| `columns` | object | Per-column configuration |
| `primary_key` | array | Primary key columns (for mutations) |

### Column Options

| Option | Type | Description |
|--------|------|-------------|
| `alias` | string | GraphQL field name |
| `exclude` | boolean | Exclude column from schema |

## Native Queries

Define custom SQL queries as GraphQL collections:

```json
{
  "native_queries": {
    "get_user_stats": {
      "sql": "SELECT user_id, count(*) as order_count, sum(total) as total_spent FROM orders WHERE user_id = {user_id:UUID} GROUP BY user_id",
      "description": "Get order statistics for a user",
      "columns": {
        "user_id": "UUID",
        "order_count": "UInt64",
        "total_spent": "Float64"
      },
      "arguments": {
        "user_id": {
          "type": "UUID",
          "description": "User ID to filter",
          "required": true
        }
      }
    },
    "daily_revenue": {
      "sql": "SELECT toDate(order_date) as date, sum(total) as revenue FROM orders WHERE order_date >= {start_date:Date} AND order_date <= {end_date:Date} GROUP BY date ORDER BY date",
      "return_type": "revenue_report",
      "arguments": {
        "start_date": { "type": "Date", "required": true },
        "end_date": { "type": "Date", "required": true }
      }
    }
  }
}
```

### SQL Parameter Syntax

Use `{param_name:Type}` for parameters:

```sql
-- Typed parameters
SELECT * FROM users WHERE id = {user_id:UUID}
SELECT * FROM events WHERE timestamp > {start:DateTime}

-- Supported types
{param:String}
{param:Int32}
{param:Int64}
{param:Float64}
{param:UUID}
{param:Date}
{param:DateTime}
{param:Bool}
```

### Native Query Options

| Option | Type | Description |
|--------|------|-------------|
| `sql` | string | SQL query with parameters |
| `description` | string | GraphQL description |
| `columns` | object | Column name → ClickHouse type mapping |
| `return_type` | string | Reference existing table type |
| `arguments` | object | Query argument definitions |

## Relationships Configuration

See [RELATIONSHIPS.md](./RELATIONSHIPS.md) for detailed relationship configuration.

```json
{
  "relationships": {
    "auto_detect": true,
    "relationships": [
      {
        "name": "author",
        "type": "object",
        "source_table": "posts",
        "target_table": "users",
        "column_mapping": { "author_id": "id" }
      }
    ]
  }
}
```

## Permissions Configuration

See [PERMISSIONS.md](./PERMISSIONS.md) for detailed permissions setup.

```json
{
  "permissions": {
    "admin_role": "admin",
    "default_role": "anonymous",
    "roles": {
      "admin": {
        "tables": {
          "users": {
            "select": { "allow_aggregations": true },
            "insert": {}
          }
        }
      }
    }
  }
}
```

## Metadata Configuration

```json
{
  "metadata": {
    "version": "1.0.0",
    "description": "Production ClickHouse connector",
    "tags": ["production", "analytics"]
  }
}
```

## Telemetry Configuration (OpenTelemetry)

Enable distributed tracing and metrics collection using OpenTelemetry:

```json
{
  "telemetry": {
    "enabled": true,
    "service_name": "ndc-clickhouse",
    "service_version": "1.0.0",
    "environment": "production",
    "tracing": {
      "enabled": true,
      "exporter": "otlp",
      "endpoint": "localhost:4317",
      "protocol": "grpc",
      "sampling_strategy": "parent_based",
      "sampling_ratio": 0.1,
      "trace_db": true,
      "trace_http": true,
      "trace_graphql": true
    },
    "metrics": {
      "enabled": true,
      "exporter": "prometheus",
      "prometheus_port": 9090,
      "prometheus_path": "/metrics",
      "metric_prefix": "ndc_clickhouse_",
      "collect_db_metrics": true,
      "collect_http_metrics": true,
      "collect_runtime_metrics": true
    }
  }
}
```

### Telemetry Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable telemetry collection |
| `service_name` | string | `"ndc-clickhouse"` | Service name for traces/metrics |
| `service_version` | string | `"1.0.0"` | Service version identifier |
| `environment` | string | `"development"` | Environment (production, staging, etc.) |

### Tracing Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable distributed tracing |
| `exporter` | string | `"otlp"` | Exporter type: `otlp`, `jaeger`, `zipkin`, `stdout`, `none` |
| `endpoint` | string | `"localhost:4317"` | Exporter endpoint URL |
| `protocol` | string | `"grpc"` | OTLP protocol: `grpc` or `http` |
| `sampling_strategy` | string | `"parent_based"` | Sampling: `always_on`, `always_off`, `trace_id_ratio`, `parent_based` |
| `sampling_ratio` | float | `1.0` | Sampling ratio (0.0-1.0) for `trace_id_ratio` strategy |
| `trace_db` | boolean | `true` | Trace database queries |
| `trace_http` | boolean | `true` | Trace HTTP requests |
| `trace_graphql` | boolean | `true` | Trace GraphQL operations |

### Metrics Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable metrics collection |
| `exporter` | string | `"prometheus"` | Exporter type: `otlp`, `prometheus`, `stdout`, `none` |
| `endpoint` | string | - | OTLP endpoint (for OTLP exporter) |
| `prometheus_port` | integer | `9090` | Port for Prometheus metrics endpoint |
| `prometheus_path` | string | `"/metrics"` | Path for Prometheus metrics endpoint |
| `metric_prefix` | string | `"ndc_clickhouse_"` | Prefix for all metric names |
| `collect_db_metrics` | boolean | `true` | Collect database query metrics |
| `collect_http_metrics` | boolean | `true` | Collect HTTP request metrics |
| `collect_runtime_metrics` | boolean | `true` | Collect Go runtime metrics |

### Exporter Examples

**OTLP (OpenTelemetry Collector)**
```json
{
  "telemetry": {
    "enabled": true,
    "tracing": {
      "enabled": true,
      "exporter": "otlp",
      "endpoint": "otel-collector:4317",
      "protocol": "grpc"
    },
    "metrics": {
      "enabled": true,
      "exporter": "otlp",
      "endpoint": "otel-collector:4317"
    }
  }
}
```

**Jaeger**
```json
{
  "telemetry": {
    "enabled": true,
    "tracing": {
      "enabled": true,
      "exporter": "jaeger",
      "endpoint": "http://jaeger:14268/api/traces"
    }
  }
}
```

**Prometheus + Zipkin**
```json
{
  "telemetry": {
    "enabled": true,
    "tracing": {
      "enabled": true,
      "exporter": "zipkin",
      "endpoint": "http://zipkin:9411/api/v2/spans"
    },
    "metrics": {
      "enabled": true,
      "exporter": "prometheus",
      "prometheus_port": 9090
    }
  }
}
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ndc_clickhouse_queries_total` | Counter | Total queries executed |
| `ndc_clickhouse_query_duration_seconds` | Histogram | Query execution duration |
| `ndc_clickhouse_query_rows_total` | Counter | Total rows returned |
| `ndc_clickhouse_query_errors_total` | Counter | Query errors |
| `ndc_clickhouse_cache_hits_total` | Counter | Cache hits |
| `ndc_clickhouse_cache_misses_total` | Counter | Cache misses |
| `ndc_clickhouse_rate_limit_total` | Counter | Rate limited requests |
| `ndc_clickhouse_http_requests_total` | Counter | HTTP requests |
| `ndc_clickhouse_http_request_duration_seconds` | Histogram | HTTP request duration |
| `ndc_clickhouse_active_connections` | Gauge | Active DB connections |
| `ndc_clickhouse_runtime_goroutines` | Gauge | Number of goroutines |
| `ndc_clickhouse_runtime_heap_alloc_bytes` | Gauge | Heap allocation |

## Cache Configuration

Enable query result caching to improve performance for repeated queries:

```json
{
  "cache": {
    "enabled": true,
    "max_size": 10000,
    "default_ttl": "5m",
    "cleanup_interval": "1m",
    "stats_enabled": true,
    "collections": {
      "users": {
        "enabled": true,
        "ttl": "10m",
        "max_size": 1000
      },
      "events": {
        "enabled": true,
        "ttl": "1m"
      },
      "sensitive_data": {
        "enabled": false
      }
    },
    "invalidation": {
      "on_mutation": true,
      "patterns": ["users", "orders"]
    }
  }
}
```

### Cache Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable query result caching |
| `max_size` | integer | `10000` | Maximum number of cached entries |
| `default_ttl` | string | `"5m"` | Default TTL for cached entries (e.g., "5m", "1h") |
| `cleanup_interval` | string | `"1m"` | Interval for cleaning up expired entries |
| `stats_enabled` | boolean | `true` | Enable cache statistics endpoint |
| `warm_on_startup` | boolean | `false` | Warm cache on startup |

### Per-Collection Cache Settings

| Option | Type | Description |
|--------|------|-------------|
| `enabled` | boolean | Enable/disable caching for this collection |
| `ttl` | string | Custom TTL for this collection |
| `max_size` | integer | Maximum entries for this collection |
| `key_prefix` | string | Cache key prefix for this collection |

### Cache Invalidation Settings

| Option | Type | Description |
|--------|------|-------------|
| `on_mutation` | boolean | Automatically invalidate cache on mutations |
| `patterns` | array | Collection patterns to invalidate on mutation |
| `webhook_url` | string | Webhook URL for external cache invalidation |

### Cache Statistics

When `stats_enabled` is true, cache statistics are available:

```json
{
  "enabled": true,
  "total_hits": 15420,
  "total_misses": 3210,
  "total_evictions": 520,
  "total_size": 8432,
  "hit_rate": 0.828,
  "main_cache": {
    "hits": 12000,
    "misses": 2800,
    "size": 7000,
    "max_size": 10000,
    "evictions": 400
  },
  "collections": {
    "users": {
      "hits": 3420,
      "misses": 410,
      "size": 1432,
      "max_size": 1000,
      "evictions": 120
    }
  }
}
```

## Rate Limiting Configuration

Configure rate limiting to protect your connector from excessive requests:

```json
{
  "rate_limiting": {
    "enabled": true,
    "type": "token_bucket",
    "default_rate": 100,
    "default_burst": 20,
    "roles": {
      "admin": {
        "rate": 1000,
        "burst": 100
      },
      "user": {
        "rate": 100,
        "burst": 20
      },
      "anonymous": {
        "rate": 10,
        "burst": 5
      }
    },
    "endpoints": {
      "/query": {
        "rate": 50,
        "burst": 10
      },
      "/mutation": {
        "rate": 20,
        "burst": 5
      }
    }
  }
}
```

### Rate Limiting Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable rate limiting |
| `type` | string | `"token_bucket"` | Rate limiter type: `token_bucket` or `leaky_bucket` |
| `default_rate` | float | `100` | Default rate limit (requests per second) |
| `default_burst` | integer | `20` | Default burst size (for token bucket) |
| `queue_size` | integer | `100` | Queue size (for leaky bucket) |

### Rate Limiter Types

**Token Bucket**
- Allows burst traffic up to the burst size
- Tokens replenish at the specified rate
- Best for: APIs with occasional bursts

**Leaky Bucket**
- Smooths out request flow
- Queues requests when rate is exceeded
- Best for: Consistent rate enforcement

### Per-Role Rate Limits

| Option | Type | Description |
|--------|------|-------------|
| `rate` | float | Requests per second for this role |
| `burst` | integer | Maximum burst size for this role |

### Per-Endpoint Rate Limits

| Option | Type | Description |
|--------|------|-------------|
| `rate` | float | Requests per second for this endpoint |
| `burst` | integer | Maximum burst size for this endpoint |

## Complete Example

```json
{
  "connection": {
    "url": "${CLICKHOUSE_URL}",
    "database": "${CLICKHOUSE_DATABASE}",
    "username": "${CLICKHOUSE_USERNAME}",
    "password": "${CLICKHOUSE_PASSWORD}",
    "max_open_conns": 20
  },
  "tables": {
    "users": {
      "alias": "Users",
      "columns": {
        "password": { "exclude": true }
      },
      "primary_key": ["id"]
    },
    "orders": {
      "alias": "Orders",
      "primary_key": ["id"]
    }
  },
  "relationships": {
    "auto_detect": true,
    "relationships": [
      {
        "name": "user",
        "type": "object",
        "source_table": "orders",
        "target_table": "users",
        "column_mapping": { "user_id": "id" }
      },
      {
        "name": "orders",
        "type": "array",
        "source_table": "users",
        "target_table": "orders",
        "column_mapping": { "id": "user_id" }
      }
    ]
  },
  "permissions": {
    "admin_role": "admin",
    "default_role": "user",
    "roles": {
      "admin": {
        "tables": {
          "users": {
            "select": { "allow_aggregations": true },
            "insert": {}
          },
          "orders": {
            "select": { "allow_aggregations": true },
            "insert": {}
          }
        }
      },
      "user": {
        "tables": {
          "orders": {
            "select": {
              "filter": {
                "user_id": { "_eq": "X-User-Id" }
              }
            }
          }
        }
      }
    }
  },
  "native_queries": {
    "my_orders": {
      "sql": "SELECT * FROM orders WHERE user_id = {user_id:UUID} ORDER BY order_date DESC",
      "return_type": "orders",
      "arguments": {
        "user_id": { "type": "UUID", "required": true }
      }
    }
  },
  "metadata": {
    "version": "1.0.0",
    "description": "E-commerce analytics connector"
  },
  "cache": {
    "enabled": true,
    "max_size": 10000,
    "default_ttl": "5m",
    "cleanup_interval": "1m",
    "stats_enabled": true,
    "collections": {
      "users": {
        "enabled": true,
        "ttl": "10m",
        "max_size": 1000
      }
    },
    "invalidation": {
      "on_mutation": true
    }
  },
  "rate_limiting": {
    "enabled": true,
    "type": "token_bucket",
    "default_rate": 100,
    "default_burst": 20,
    "roles": {
      "admin": { "rate": 1000, "burst": 100 },
      "user": { "rate": 100, "burst": 20 }
    }
  }
}
```

## Validating Configuration

```bash
# Validate your configuration
./bin/ndc-clickhouse validate --config ./config

# Output GraphQL schema
./bin/ndc-clickhouse print-schema --config ./config
```
