# Getting Started with NDC ClickHouse Go

This guide will help you get up and running with the ClickHouse GraphQL connector in minutes.

## Prerequisites

- Go 1.21 or later
- Docker and Docker Compose (optional, for local development)
- Access to a ClickHouse database

## Quick Start (5 minutes)

### Option 1: Using Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/your-org/ndc-clickhouse-go.git
cd ndc-clickhouse-go

# Start ClickHouse and the connector
docker-compose up -d

# The connector is now available at http://localhost:8080
```

### Option 2: Building from Source

```bash
# Clone the repository
git clone https://github.com/your-org/ndc-clickhouse-go.git
cd ndc-clickhouse-go

# Install dependencies
make deps

# Build the binary
make build

# Run the connector
export CLICKHOUSE_URL="clickhouse://localhost:9000"
export CLICKHOUSE_DATABASE="default"
export CLICKHOUSE_USERNAME="default"
export CLICKHOUSE_PASSWORD=""

./bin/ndc-clickhouse serve --configuration ./config
```

## Step-by-Step Setup

### Step 1: Connect to Your ClickHouse Database

First, introspect your database to generate the configuration:

```bash
./bin/ndc-clickhouse introspect \
  --url "clickhouse://your-host:9000" \
  --database "your_database" \
  --username "your_user" \
  --password "your_password" \
  --output ./config
```

This creates:
- `config/configuration.json` - The main configuration file
- `config/schema.md` - Documentation of your schema

### Step 2: Review and Customize Configuration

Edit `config/configuration.json` to:
- Add table aliases for cleaner GraphQL names
- Configure relationships between tables
- Set up permissions for different roles
- Add native queries for complex operations

Example configuration:

```json
{
  "connection": {
    "url": "${CLICKHOUSE_URL}",
    "database": "${CLICKHOUSE_DATABASE}",
    "username": "${CLICKHOUSE_USERNAME}",
    "password": "${CLICKHOUSE_PASSWORD}"
  },
  "tables": {
    "user_accounts": {
      "alias": "Users"
    },
    "product_catalog": {
      "alias": "Products"
    }
  },
  "relationships": {
    "relationships": [
      {
        "name": "orders",
        "type": "array",
        "source_table": "users",
        "target_table": "orders",
        "column_mapping": {
          "id": "user_id"
        }
      }
    ]
  }
}
```

### Step 3: Start the Connector

```bash
./bin/ndc-clickhouse serve --configuration ./config
```

The connector will:
1. Connect to your ClickHouse database
2. Introspect the schema
3. Start serving the NDC API on port 8080

### Step 4: Test the API

**Get Schema:**
```bash
curl http://localhost:8080/schema
```

**Execute a Query:**
```bash
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "users",
    "query": {
      "fields": {
        "id": {"type": "column", "column": "id"},
        "name": {"type": "column", "column": "name"}
      },
      "limit": 10
    }
  }'
```

## Connecting to Hasura

### Hasura v3 (DDN)

Add the connector to your Hasura DDN project:

```bash
# In your Hasura project directory
hasura connector add clickhouse \
  --url http://ndc-clickhouse:8080 \
  --configuration ./connectors/clickhouse
```

### Hasura v2

Add as a data source in the Hasura Console or via metadata:

```yaml
# metadata/databases/clickhouse.yaml
- name: clickhouse
  kind: native_data_connector
  configuration:
    uri:
      from_env: NDC_CLICKHOUSE_URL
```

## Next Steps

- [Configuration Guide](./CONFIGURATION.md) - Detailed configuration options
- [Relationships Guide](./RELATIONSHIPS.md) - Setting up table relationships
- [Permissions Guide](./PERMISSIONS.md) - Row-level security
- [Native Queries Guide](./NATIVE_QUERIES.md) - Custom SQL queries
- [API Reference](./API_REFERENCE.md) - Complete API documentation

## Troubleshooting

### Connection Issues

```bash
# Test connection
./bin/ndc-clickhouse validate --config ./config

# Check ClickHouse connectivity
clickhouse-client -h localhost -q "SELECT 1"
```

### Common Errors

| Error | Solution |
|-------|----------|
| `connection refused` | Check if ClickHouse is running |
| `authentication failed` | Verify username/password |
| `database not found` | Check database name |
| `table not found` | Re-run introspect command |

### Getting Help

- Check the [FAQ](./FAQ.md)
- Open an issue on GitHub
- Join our community Discord
