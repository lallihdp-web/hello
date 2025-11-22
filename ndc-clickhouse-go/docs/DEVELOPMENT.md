# Development Guide

This guide helps developers understand and contribute to the NDC ClickHouse connector.

## Project Structure

```
ndc-clickhouse-go/
├── cmd/
│   └── ndc-clickhouse/         # CLI entrypoint
│       ├── main.go             # Main entry point
│       └── cli.go              # CLI commands (introspect, validate, etc.)
│
├── connector/                   # NDC connector implementation
│   ├── connector.go            # Main connector struct and interface
│   ├── query.go                # Query execution
│   └── mutation.go             # Mutation execution
│
├── clickhouse/                  # ClickHouse client
│   └── client.go               # Connection and query execution
│
├── schema/                      # GraphQL schema generation
│   ├── types.go                # Type mapping (ClickHouse → GraphQL)
│   └── types_test.go           # Type tests
│
├── config/                      # Configuration handling
│   ├── config.go               # Main configuration
│   ├── relationships.go        # Relationship definitions
│   ├── permissions.go          # Permission system
│   └── *_test.go               # Tests
│
├── internal/
│   └── query/                   # Query building
│       ├── builder.go          # SQL query builder
│       ├── executor.go         # Query execution
│       └── builder_test.go     # Tests
│
├── tests/                       # Integration tests
│   └── integration_test.go
│
├── docs/                        # Documentation
│
├── config/                      # Sample configuration
│   └── configuration.json
│
├── init-db/                     # Sample ClickHouse init scripts
│   └── 01-create-tables.sql
│
├── Dockerfile
├── docker-compose.yaml
├── Makefile
├── go.mod
└── README.md
```

## Architecture

### Component Flow

```
┌──────────────────────────────────────────────────────────────┐
│                      GraphQL Request                          │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                     NDC SDK (HTTP Server)                     │
│  - Handles HTTP requests                                      │
│  - Validates NDC protocol                                     │
│  - Calls connector methods                                    │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                      Connector Layer                          │
│  connector/connector.go                                       │
│  - ParseConfiguration: Load and validate config               │
│  - TryInitState: Connect to ClickHouse, build schema          │
│  - GetSchema: Return NDC schema                               │
│  - Query: Execute read queries                                │
│  - Mutation: Execute write operations                         │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                    Query Builder Layer                        │
│  internal/query/builder.go                                    │
│  - Translates NDC query → ClickHouse SQL                      │
│  - Handles filtering, sorting, pagination                     │
│  - Builds aggregations                                        │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                    ClickHouse Client                          │
│  clickhouse/client.go                                         │
│  - Connection management                                      │
│  - Query execution                                            │
│  - Result mapping                                             │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                     ClickHouse Database                       │
└──────────────────────────────────────────────────────────────┘
```

## Key Interfaces

### Connector Interface

The connector implements the `ndc-sdk-go` Connector interface:

```go
type Connector interface {
    // Configuration
    ParseConfiguration(ctx, configDir) (*Config, error)
    TryInitState(ctx, config, metrics) (*State, error)

    // Schema
    GetCapabilities(config) *CapabilitiesResponse
    GetSchema(ctx, config, state) (SchemaResponse, error)

    // Queries
    Query(ctx, config, state, request) (QueryResponse, error)
    QueryExplain(ctx, config, state, request) (*ExplainResponse, error)

    // Mutations
    Mutation(ctx, config, state, request) (*MutationResponse, error)
    MutationExplain(ctx, config, state, request) (*ExplainResponse, error)

    // Health
    HealthCheck(ctx, config, state) error
}
```

### Query Builder Interface

```go
type Builder struct {
    collection string
    fields     map[string]Field
    where      *Expression
    orderBy    *OrderBy
    limit      *int
    offset     *int
    aggregates map[string]Aggregate
}

func (b *Builder) Build() (*BuildResult, error)
```

## Development Setup

### Prerequisites

```bash
# Install Go 1.21+
go version

# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/cosmtrek/air@latest  # Hot reload
```

### Running Locally

```bash
# Start ClickHouse
docker-compose up -d clickhouse

# Install dependencies
make deps

# Build
make build

# Run with hot reload
make dev
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires ClickHouse)
go test -tags=integration ./tests/...

# With coverage
make test-coverage
```

## Adding New Features

### Adding a New ClickHouse Type

1. Add the type constant in `schema/types.go`:
```go
const ScalarNewType = "NewType"
```

2. Update `ParseClickHouseType` to handle the type:
```go
if strings.HasPrefix(typeStr, "NewType") {
    info.BaseType = ScalarNewType
    return info
}
```

3. Update `clickHouseToScalarName` for GraphQL mapping:
```go
case ScalarNewType:
    return "NewGraphQLType"
```

4. Add to `GetScalarTypes()` if it's a new scalar:
```go
"NewGraphQLType": schema.ScalarType{
    ComparisonOperators: ...,
    AggregateFunctions: ...,
    Representation: ...,
},
```

5. Write tests in `schema/types_test.go`

### Adding a New Comparison Operator

1. Update `internal/query/builder.go`:
```go
func (b *Builder) mapOperator(op string) string {
    switch op {
    // ... existing cases
    case "_new_op":
        return "NEW_SQL_OP"
    }
}
```

2. Update `buildBinaryComparison` for special handling if needed

3. Add to schema in `schema/types.go` under the appropriate scalar type

### Adding a New CLI Command

1. Add command struct in `cmd/ndc-clickhouse/cli.go`:
```go
type NewCommand struct {
    Option string `help:"Description"`
}

func (cmd *NewCommand) Run() error {
    // Implementation
}
```

2. Add to main.go switch statement:
```go
case "new-command":
    runNewCommand()
    return
```

3. Add to help text in `printHelp()`

## Code Style

### Formatting

```bash
# Format code
make fmt

# Run linter
make lint
```

### Guidelines

1. **Error Handling**: Always wrap errors with context
```go
if err != nil {
    return nil, fmt.Errorf("failed to do X: %w", err)
}
```

2. **Documentation**: Add comments for exported functions
```go
// BuildQuery constructs a ClickHouse SQL query from an NDC query request.
// It handles field selection, filtering, sorting, and pagination.
func BuildQuery(request *schema.QueryRequest) (*BuildResult, error) {
```

3. **Testing**: Write tests for new functionality
```go
func TestNewFeature(t *testing.T) {
    // Arrange
    input := ...

    // Act
    result := NewFeature(input)

    // Assert
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

## Debugging

### Enable Debug Logging

```bash
export HASURA_LOG_LEVEL=debug
./bin/ndc-clickhouse serve --configuration ./config
```

### Query Explain

```bash
curl -X POST http://localhost:8080/query/explain \
  -H "Content-Type: application/json" \
  -d '{ "collection": "users", "query": { ... } }'
```

### ClickHouse Query Log

```sql
-- In ClickHouse
SELECT query, query_duration_ms
FROM system.query_log
WHERE query LIKE '%your_table%'
ORDER BY event_time DESC
LIMIT 10;
```

## Performance Considerations

1. **Connection Pooling**: Configure appropriate pool sizes
2. **Query Optimization**: Use indexes, avoid SELECT *
3. **Batch Operations**: Use batch inserts for mutations
4. **Caching**: Schema is cached in state

## Release Process

1. Update version in code
2. Update CHANGELOG.md
3. Run full test suite
4. Build and tag release
5. Push Docker image

```bash
VERSION=1.0.0
git tag v$VERSION
git push origin v$VERSION
make docker-build VERSION=$VERSION
docker push your-org/ndc-clickhouse:$VERSION
```
