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
                "user_id": { "_eq": "X-Hasura-User-Id" }
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
