# Testing Guide

Complete guide for testing the NDC ClickHouse connector.

## Table of Contents

1. [Running Tests](#running-tests)
2. [Test Structure](#test-structure)
3. [Writing Tests](#writing-tests)
4. [Benchmarking](#benchmarking)
5. [Integration Testing](#integration-testing)
6. [Test Coverage](#test-coverage)

## Running Tests

### Run All Tests

```bash
# Run all unit tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./cache/...
go test -v ./analytics/...
go test -v ./middleware/...
go test -v ./config/...
```

### Run Tests with Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
```

### Run Benchmarks

```bash
# Run all benchmarks
go test -bench=. ./...

# Run benchmarks with memory allocation stats
go test -bench=. -benchmem ./...

# Run specific benchmark
go test -bench=BenchmarkInMemoryCache_Set ./cache/...

# Run benchmark multiple times for accuracy
go test -bench=. -count=5 ./...
```

## Test Structure

### Directory Layout

```
ndc-clickhouse-go/
├── cache/
│   ├── cache.go
│   └── cache_test.go        # Unit tests for cache
├── analytics/
│   ├── logger.go
│   └── logger_test.go       # Unit tests for analytics
├── middleware/
│   ├── auth.go
│   ├── auth_test.go         # Unit tests for auth
│   ├── ratelimit.go
│   └── ratelimit_test.go    # Unit tests for rate limiting
├── config/
│   ├── config.go
│   ├── config_test.go       # Unit tests for config
│   ├── permissions.go
│   └── permissions_test.go  # Unit tests for permissions
├── internal/
│   └── query/
│       ├── builder.go
│       └── builder_test.go  # Unit tests for query builder
└── tests/
    └── integration_test.go  # Integration tests
```

### Test Naming Convention

```go
// Unit test functions
func TestTypeName_MethodName(t *testing.T) {}
func TestTypeName_MethodName_Scenario(t *testing.T) {}

// Table-driven test
func TestTypeName_MethodName(t *testing.T) {
    tests := []struct {
        name     string
        input    InputType
        expected OutputType
    }{
        {"scenario1", input1, expected1},
        {"scenario2", input2, expected2},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}

// Benchmark functions
func BenchmarkTypeName_MethodName(b *testing.B) {}
```

## Writing Tests

### Unit Test Example

```go
package cache

import (
    "testing"
    "time"
)

func TestInMemoryCache_SetAndGet(t *testing.T) {
    // Setup
    cache := NewInMemoryCache(100)

    // Test Set
    cache.Set("key1", "value1", time.Minute)

    // Test Get
    value, found := cache.Get("key1")

    // Assertions
    if !found {
        t.Fatal("expected to find cached value")
    }
    if value != "value1" {
        t.Errorf("expected value1, got %v", value)
    }
}

func TestInMemoryCache_Expiration(t *testing.T) {
    cache := NewInMemoryCache(100)

    // Set with short TTL
    cache.Set("key1", "value1", 10*time.Millisecond)

    // Should be found immediately
    _, found := cache.Get("key1")
    if !found {
        t.Error("expected to find value before expiration")
    }

    // Wait for expiration
    time.Sleep(20 * time.Millisecond)

    // Should be expired
    _, found = cache.Get("key1")
    if found {
        t.Error("expected value to be expired")
    }
}
```

### Table-Driven Test Example

```go
func TestPermissionChecker_CanSelect(t *testing.T) {
    tests := []struct {
        name     string
        role     string
        table    string
        perms    *PermissionsConfig
        expected bool
    }{
        {
            name:  "admin can access all",
            role:  "admin",
            table: "users",
            perms: &PermissionsConfig{
                AdminRoles: []string{"admin"},
            },
            expected: true,
        },
        {
            name:  "user with permission",
            role:  "user",
            table: "public_data",
            perms: &PermissionsConfig{
                Roles: map[string]*RolePermission{
                    "user": {
                        Tables: map[string]*TablePermission{
                            "public_data": {Select: &SelectPermission{Allowed: true}},
                        },
                    },
                },
            },
            expected: true,
        },
        {
            name:  "user without permission",
            role:  "user",
            table: "private_data",
            perms: &PermissionsConfig{},
            expected: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            checker := NewPermissionChecker(tt.perms)
            result := checker.CanSelect(tt.role, tt.table)
            if result != tt.expected {
                t.Errorf("CanSelect(%s, %s) = %v, want %v",
                    tt.role, tt.table, result, tt.expected)
            }
        })
    }
}
```

### HTTP Handler Test Example

```go
func TestRateLimiter_Middleware(t *testing.T) {
    limiter := NewRateLimiter(&RateLimitConfig{
        Enabled: true,
        Default: &RateLimit{
            Limit:  1,
            Window: time.Second,
            Burst:  1,
        },
        RoleLimits: map[string]*RateLimit{
            "anonymous": {Limit: 1, Window: time.Second, Burst: 1},
        },
    })

    handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    // First request should succeed
    req := httptest.NewRequest("GET", "/test", nil)
    req.RemoteAddr = "127.0.0.1:1234"
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }

    // Second request should be rate limited
    w = httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusTooManyRequests {
        t.Errorf("expected 429, got %d", w.Code)
    }
}
```

## Benchmarking

### Basic Benchmark

```go
func BenchmarkInMemoryCache_Set(b *testing.B) {
    cache := NewInMemoryCache(10000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cache.Set(string(rune(i%1000)), i, time.Minute)
    }
}

func BenchmarkInMemoryCache_Get(b *testing.B) {
    cache := NewInMemoryCache(10000)

    // Pre-populate
    for i := 0; i < 1000; i++ {
        cache.Set(string(rune(i)), i, time.Minute)
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cache.Get(string(rune(i % 1000)))
    }
}
```

### Concurrent Benchmark

```go
func BenchmarkInMemoryCache_Concurrent(b *testing.B) {
    cache := NewInMemoryCache(10000)

    b.RunParallel(func(pb *testing.PB) {
        i := 0
        for pb.Next() {
            key := string(rune(i % 1000))
            if i%2 == 0 {
                cache.Set(key, i, time.Minute)
            } else {
                cache.Get(key)
            }
            i++
        }
    })
}
```

### Benchmark Results Interpretation

```
BenchmarkInMemoryCache_Set-16      8404808    138.0 ns/op    16 B/op    2 allocs/op
│                          │       │          │              │          │
│                          │       │          │              │          └── allocations per op
│                          │       │          │              └── bytes allocated per op
│                          │       │          └── nanoseconds per operation
│                          │       └── number of iterations
│                          └── number of CPUs used
└── benchmark name
```

## Integration Testing

### Setup with Docker

```go
// tests/integration_test.go
// +build integration

package tests

import (
    "context"
    "os"
    "testing"

    "github.com/your-org/ndc-clickhouse-go/clickhouse"
)

var testClient *clickhouse.Client

func TestMain(m *testing.M) {
    // Setup
    url := os.Getenv("CLICKHOUSE_TEST_URL")
    if url == "" {
        url = "clickhouse://localhost:9000"
    }

    client, err := clickhouse.NewClient(url, "test")
    if err != nil {
        panic(err)
    }
    testClient = client

    // Create test tables
    setupTestDatabase()

    // Run tests
    code := m.Run()

    // Cleanup
    cleanupTestDatabase()

    os.Exit(code)
}

func setupTestDatabase() {
    ctx := context.Background()
    testClient.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS test_users (
            id UInt64,
            name String,
            created_at DateTime
        ) ENGINE = MergeTree() ORDER BY id
    `)
}

func cleanupTestDatabase() {
    ctx := context.Background()
    testClient.Exec(ctx, "DROP TABLE IF EXISTS test_users")
}
```

### Run Integration Tests

```bash
# Start ClickHouse
docker run -d --name clickhouse-test -p 9000:9000 clickhouse/clickhouse-server

# Run integration tests
CLICKHOUSE_TEST_URL=clickhouse://localhost:9000 go test -tags=integration ./tests/...

# Cleanup
docker stop clickhouse-test && docker rm clickhouse-test
```

## Test Coverage

### Coverage Goals

| Package | Target Coverage |
|---------|-----------------|
| cache | 90%+ |
| analytics | 85%+ |
| middleware | 85%+ |
| config | 80%+ |
| internal/query | 75%+ |
| connector | 70%+ |

### Check Coverage

```bash
# Generate coverage for all packages
go test -coverprofile=coverage.out ./...

# View coverage summary
go tool cover -func=coverage.out | grep total

# View per-function coverage
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

### CI Coverage Check

```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test -coverprofile=coverage.out ./...
      - run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Coverage $COVERAGE% is below threshold 70%"
            exit 1
          fi
```

## Mock Objects

### Creating Mocks

```go
// mocks/clickhouse_mock.go
package mocks

type MockClickHouseClient struct {
    QueryFunc func(ctx context.Context, sql string) (driver.Rows, error)
    ExecFunc  func(ctx context.Context, sql string) error
}

func (m *MockClickHouseClient) Query(ctx context.Context, sql string, args ...interface{}) (driver.Rows, error) {
    if m.QueryFunc != nil {
        return m.QueryFunc(ctx, sql)
    }
    return nil, nil
}

func (m *MockClickHouseClient) Exec(ctx context.Context, sql string, args ...interface{}) error {
    if m.ExecFunc != nil {
        return m.ExecFunc(ctx, sql)
    }
    return nil
}
```

### Using Mocks in Tests

```go
func TestQueryHandler_WithMock(t *testing.T) {
    mockClient := &mocks.MockClickHouseClient{
        QueryFunc: func(ctx context.Context, sql string) (driver.Rows, error) {
            // Return mock data
            return &mockRows{
                columns: []string{"id", "name"},
                data: [][]interface{}{
                    {1, "Alice"},
                    {2, "Bob"},
                },
            }, nil
        },
    }

    handler := NewQueryHandler(mockClient)
    result, err := handler.Execute(ctx, queryRequest)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(result.Rows) != 2 {
        t.Errorf("expected 2 rows, got %d", len(result.Rows))
    }
}
```

## Performance Testing

### Load Testing with hey

```bash
# Install hey
go install github.com/rakyll/hey@latest

# Test query endpoint
hey -n 1000 -c 50 -m POST \
    -H "Content-Type: application/json" \
    -d '{"collection":"users","query":{"fields":{"id":{"type":"column","column":"id"}}}}' \
    http://localhost:8080/query

# Test with authentication
hey -n 1000 -c 50 -m POST \
    -H "Content-Type: application/json" \
    -H "X-API-Key: your-api-key" \
    -d '{"sql":"SELECT 1"}' \
    http://localhost:3000/api/query
```

### Profiling

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./cache/...
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=. ./cache/...
go tool pprof mem.prof

# Block profiling (concurrency)
go test -blockprofile=block.prof -bench=. ./cache/...
go tool pprof block.prof
```
