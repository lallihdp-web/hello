# NDC ClickHouse Go Connector

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/)

A Native Data Connector for [Hasura DDN](https://hasura.io/ddn) that provides **instant GraphQL APIs** over ClickHouse databases. Built with Go using the [NDC SDK for Go](https://github.com/hasura/ndc-sdk-go).

**Just like Hasura** - auto-generate GraphQL from your database with zero code. Point it at your ClickHouse instance and get a full GraphQL API instantly.

## Key Features

| Feature | Description |
|---------|-------------|
| **Auto Schema Discovery** | Automatically introspects tables and columns |
| **Zero Code GraphQL** | Instant GraphQL API without writing resolvers |
| **Full Query Support** | Filtering, sorting, pagination out of the box |
| **Aggregations** | COUNT, SUM, AVG, MIN, MAX and custom aggregates |
| **Relationships** | Auto-detected JOINs between tables |
| **Row-Level Security** | Fine-grained permissions per role |
| **Native Queries** | Custom SQL as GraphQL collections |
| **Mutations** | INSERT support via batch operations |
| **Rate Limiting** | Token bucket & leaky bucket algorithms |
| **Caching** | LRU cache with TTL for query results |
| **OpenTelemetry** | Distributed tracing and metrics |
| **Web Console** | Built-in GraphQL playground |
| **CLI Tools** | Introspect, validate, and manage configuration |

## Quick Start (2 minutes)

### Option 1: Docker (Recommended)

```bash
git clone https://github.com/your-org/ndc-clickhouse-go.git
cd ndc-clickhouse-go

# Start ClickHouse and connector
docker-compose up -d

# GraphQL API available at http://localhost:8080
```

### Option 2: From Source

```bash
# Build
make deps && make build

# Introspect your database
./bin/ndc-clickhouse introspect \
  --url "clickhouse://localhost:9000" \
  --database "your_db" \
  --output ./config

# Start the connector
./bin/ndc-clickhouse serve --configuration ./config
```

## How It Works

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  GraphQL Query  │ ──▶ │  NDC Connector   │ ──▶ │   ClickHouse    │
│                 │     │  (This project)  │     │    Database     │
│  users {        │     │                  │     │                 │
│    id           │     │  - Schema Gen    │     │  SELECT id,     │
│    name         │     │  - Query Build   │     │  name FROM      │
│    orders {     │     │  - Permissions   │     │  users ...      │
│      total      │     │  - Results Map   │     │                 │
│    }            │     │                  │     │                 │
│  }              │     │                  │     │                 │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

## GraphQL Examples

### Query with Filtering
```graphql
query {
  users(where: { is_active: { _eq: true }, age: { _gte: 18 } }) {
    id
    name
    email
    orders(limit: 5, order_by: { created_at: desc }) {
      id
      total
      status
    }
  }
}
```

### Aggregations
```graphql
query {
  orders_aggregate(where: { status: { _eq: "completed" } }) {
    aggregate {
      count
      sum { total }
      avg { total }
    }
  }
}
```

### Insert Mutation
```graphql
mutation {
  insert_users(objects: [
    { name: "Alice", email: "alice@example.com" }
  ]) {
    affected_rows
  }
}
```

## Configuration

```json
{
  "connection": {
    "url": "${CLICKHOUSE_URL}",
    "database": "${CLICKHOUSE_DATABASE}"
  },
  "tables": {
    "users": { "alias": "Users" }
  },
  "relationships": {
    "auto_detect": true
  },
  "permissions": {
    "admin_role": "admin",
    "roles": {
      "user": {
        "tables": {
          "orders": {
            "select": {
              "filter": { "user_id": { "_eq": "X-User-Id" } }
            }
          }
        }
      }
    }
  },
  "telemetry": {
    "enabled": true,
    "service_name": "ndc-clickhouse",
    "tracing": {
      "enabled": true,
      "exporter": "otlp",
      "endpoint": "localhost:4317"
    },
    "metrics": {
      "enabled": true,
      "exporter": "prometheus",
      "prometheus_port": 9090
    }
  }
}
```

## CLI Commands

```bash
# Introspect database and generate config
ndc-clickhouse introspect --url clickhouse://localhost:9000 --database mydb

# Validate configuration
ndc-clickhouse validate --config ./config

# Print GraphQL schema
ndc-clickhouse print-schema --config ./config

# Start server
ndc-clickhouse serve --configuration ./config
```

## Supported ClickHouse Types

| ClickHouse | GraphQL | Operators |
|------------|---------|-----------|
| Int8-Int256, UInt8-UInt256 | Int/BigInt | `_eq`, `_neq`, `_gt`, `_gte`, `_lt`, `_lte`, `_in` |
| Float32, Float64, Decimal | Float | Same as Int |
| String, FixedString | String | Same + `_like`, `_ilike`, `_regex` |
| Bool | Boolean | `_eq`, `_neq` |
| UUID | UUID | `_eq`, `_neq`, `_in` |
| Date, DateTime | Date/DateTime | All comparison operators |
| Array(T) | [T] | Contains operators |
| Nullable(T) | T (nullable) | `_is_null` |
| Enum, Map, Tuple | String/JSON | Varies |

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/GETTING_STARTED.md) | Quick start guide |
| [Configuration](docs/CONFIGURATION.md) | Full configuration reference (includes telemetry) |
| [Relationships](docs/RELATIONSHIPS.md) | Setting up table relationships |
| [Permissions](docs/PERMISSIONS.md) | Row-level security setup |
| [Architecture](docs/ARCHITECTURE.md) | Technical architecture overview |
| [API Reference](docs/API_REFERENCE.md) | Package APIs and endpoints |
| [Testing](docs/TESTING.md) | Test and benchmark guide |
| [Development](docs/DEVELOPMENT.md) | Developer guide |
| [Contributing](CONTRIBUTING.md) | How to contribute |

## Project Structure

```
ndc-clickhouse-go/
├── cmd/ndc-clickhouse/     # CLI entrypoint & commands
├── connector/              # NDC interface implementation
├── clickhouse/             # ClickHouse client & introspection
├── schema/                 # Type mapping (ClickHouse → GraphQL)
├── config/                 # Configuration & permissions
├── internal/query/         # SQL query builder
├── middleware/             # Auth, rate limiting (token/leaky bucket)
├── telemetry/              # OpenTelemetry tracing & metrics
├── cache/                  # LRU query cache
├── analytics/              # Query logging & stats
├── subscription/           # Subscription support
├── console/                # Web UI & GraphQL playground
├── tests/                  # Integration tests
├── docs/                   # Documentation
├── Dockerfile
├── docker-compose.yaml
└── Makefile
```

## Development

```bash
# Run tests
make test

# Run with hot reload
make dev

# Run linter
make lint

# Build Docker image
make docker-build
```

## Comparison with Hasura's Rust Connector

This Go implementation is inspired by [hasura/ndc-clickhouse](https://github.com/hasura/ndc-clickhouse) (Rust) but offers:

- **Go ecosystem**: Easier to extend for Go teams
- **Single binary**: No Rust toolchain needed
- **Same features**: Full NDC spec compliance
- **Added**: CLI introspection tools, enhanced permissions

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

Built with ❤️ using [Hasura NDC SDK for Go](https://github.com/hasura/ndc-sdk-go)
