# NDC ClickHouse Go Connector

A Native Data Connector for [Hasura DDN](https://hasura.io/ddn) that provides instant GraphQL APIs over ClickHouse databases, built with Go using the [NDC SDK for Go](https://github.com/hasura/ndc-sdk-go).

## Features

- **Auto Schema Introspection**: Automatically discovers tables and columns from ClickHouse
- **Type Mapping**: Maps ClickHouse types to GraphQL types
- **Query Support**: Full query capabilities with filtering, sorting, pagination
- **Aggregations**: Support for COUNT, SUM, AVG, MIN, MAX and custom aggregates
- **Native Queries**: Define raw SQL queries as virtual GraphQL collections
- **Mutations**: INSERT support via batch operations
- **Environment Variables**: Configuration supports env var substitution

## Quick Start

### Using Docker Compose

```bash
# Start ClickHouse and the connector
docker-compose up -d

# The connector will be available at http://localhost:8080
```

### Building from Source

```bash
# Install dependencies
make deps

# Build the binary
make build

# Run the connector
make run
```

## Configuration

Create a `configuration.json` in your config directory:

```json
{
  "connection": {
    "url": "${CLICKHOUSE_URL}",
    "username": "${CLICKHOUSE_USERNAME}",
    "password": "${CLICKHOUSE_PASSWORD}",
    "database": "default",
    "secure": false
  },
  "tables": {
    "users": {
      "alias": "Users"
    }
  },
  "native_queries": {
    "get_stats": {
      "sql": "SELECT count(*) as total FROM events WHERE date >= {start:Date}",
      "columns": {
        "total": "UInt64"
      },
      "arguments": {
        "start": {
          "type": "Date",
          "required": true
        }
      }
    }
  }
}
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLICKHOUSE_URL` | ClickHouse connection URL | `clickhouse://localhost:9000` |
| `CLICKHOUSE_DATABASE` | Database name | `default` |
| `CLICKHOUSE_USERNAME` | Username | `default` |
| `CLICKHOUSE_PASSWORD` | Password | - |
| `HASURA_CONNECTOR_PORT` | Connector HTTP port | `8080` |
| `HASURA_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

## GraphQL Query Examples

### Basic Query
```graphql
query {
  users(limit: 10) {
    id
    name
    email
    created_at
  }
}
```

### Filtering
```graphql
query {
  users(where: { is_active: { _eq: true }, age: { _gte: 18 } }) {
    id
    name
  }
}
```

### Sorting and Pagination
```graphql
query {
  products(
    order_by: { price: desc }
    limit: 10
    offset: 20
  ) {
    name
    price
  }
}
```

### Aggregations
```graphql
query {
  orders_aggregate {
    aggregate {
      count
      sum {
        total_price
      }
      avg {
        quantity
      }
    }
  }
}
```

## Supported ClickHouse Types

| ClickHouse Type | GraphQL Type |
|-----------------|--------------|
| Int8, Int16, Int32, UInt8, UInt16, UInt32 | Int |
| Int64, UInt64, Int128, Int256 | BigInt |
| Float32, Float64, Decimal | Float |
| String, FixedString | String |
| Bool | Boolean |
| UUID | UUID |
| Date, Date32 | Date |
| DateTime, DateTime64 | DateTime |
| Array(T) | [T] |
| Nullable(T) | T (nullable) |
| Enum | String |
| Map, Tuple, Nested, JSON | JSON |

## Comparison Operators

| Operator | Description |
|----------|-------------|
| `_eq` | Equal |
| `_neq` | Not equal |
| `_gt` | Greater than |
| `_gte` | Greater than or equal |
| `_lt` | Less than |
| `_lte` | Less than or equal |
| `_in` | In array |
| `_like` | SQL LIKE pattern |
| `_ilike` | Case-insensitive LIKE |
| `_regex` | Regular expression match |
| `_is_null` | Is null check |

## Development

```bash
# Run tests
make test

# Run with hot reload
make dev

# Format code
make fmt

# Run linter
make lint
```

## Architecture

```
ndc-clickhouse-go/
├── cmd/ndc-clickhouse/     # CLI entrypoint
├── connector/              # NDC connector implementation
├── clickhouse/             # ClickHouse client and introspection
├── schema/                 # Type mapping and NDC schema
├── internal/query/         # SQL query builder
└── config/                 # Configuration handling
```

## License

Apache License 2.0
