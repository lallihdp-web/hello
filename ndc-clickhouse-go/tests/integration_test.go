// +build integration

package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
	"github.com/your-org/ndc-clickhouse-go/config"
)

// Integration tests require a running ClickHouse instance
// Run with: go test -tags=integration ./tests/...

func getTestClient(t *testing.T) *clickhouse.Client {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		url = "clickhouse://localhost:9000"
	}

	database := os.Getenv("CLICKHOUSE_DATABASE")
	if database == "" {
		database = "default"
	}

	cfg := &config.ConnectionConfig{
		URL:      url,
		Database: database,
		Username: os.Getenv("CLICKHOUSE_USERNAME"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	}

	if cfg.Username == "" {
		cfg.Username = "default"
	}

	client, err := clickhouse.NewClient(cfg)
	if err != nil {
		t.Skipf("Could not connect to ClickHouse: %v", err)
	}

	return client
}

func TestIntegration_Connection(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test simple query
	rows, err := client.Query(ctx, "SELECT 1 AS result")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("Expected at least one row")
	}
}

func TestIntegration_GetTables(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tables, err := client.GetTables(ctx)
	if err != nil {
		t.Fatalf("GetTables failed: %v", err)
	}

	t.Logf("Found %d tables", len(tables))

	for _, table := range tables {
		t.Logf("  - %s (%s)", table.Name, table.Engine)
	}
}

func TestIntegration_GetColumns(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First get a table name
	tables, err := client.GetTables(ctx)
	if err != nil {
		t.Fatalf("GetTables failed: %v", err)
	}

	if len(tables) == 0 {
		t.Skip("No tables found for column test")
	}

	tableName := tables[0].Name
	columns, err := client.GetColumns(ctx, tableName)
	if err != nil {
		t.Fatalf("GetColumns failed: %v", err)
	}

	t.Logf("Columns for %s:", tableName)
	for _, col := range columns {
		t.Logf("  - %s: %s (PK: %v)", col.Name, col.Type, col.IsPrimaryKey)
	}
}

func TestIntegration_GetAllColumns(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allColumns, err := client.GetAllColumns(ctx)
	if err != nil {
		t.Fatalf("GetAllColumns failed: %v", err)
	}

	t.Logf("Found columns for %d tables", len(allColumns))
}

func TestIntegration_CreateTestTable(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a test table
	createSQL := `
		CREATE TABLE IF NOT EXISTS test_integration (
			id UUID DEFAULT generateUUIDv4(),
			name String,
			value Int64,
			created_at DateTime DEFAULT now()
		) ENGINE = MergeTree()
		ORDER BY (created_at, id)
	`

	err := client.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Create table failed: %v", err)
	}

	// Insert test data
	insertSQL := `INSERT INTO test_integration (name, value) VALUES (?, ?)`
	err = client.Exec(ctx, insertSQL, "test1", 100)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Query the data
	rows, err := client.Query(ctx, "SELECT name, value FROM test_integration WHERE name = ?", "test1")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("Expected at least one row")
	}

	var name string
	var value int64
	if err := rows.Scan(&name, &value); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if name != "test1" || value != 100 {
		t.Errorf("Got name=%q value=%d, want name=test1 value=100", name, value)
	}

	// Cleanup
	err = client.Exec(ctx, "DROP TABLE IF EXISTS test_integration")
	if err != nil {
		t.Logf("Warning: Failed to drop test table: %v", err)
	}
}

func TestIntegration_BatchInsert(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create test table
	createSQL := `
		CREATE TABLE IF NOT EXISTS test_batch (
			id UInt64,
			name String,
			value Float64
		) ENGINE = MergeTree()
		ORDER BY id
	`

	err := client.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Create table failed: %v", err)
	}
	defer client.Exec(ctx, "DROP TABLE IF EXISTS test_batch")

	// Prepare batch
	batch, err := client.PrepareBatch(ctx, "INSERT INTO test_batch (id, name, value)")
	if err != nil {
		t.Fatalf("PrepareBatch failed: %v", err)
	}

	// Add rows
	for i := 0; i < 100; i++ {
		err := batch.Append(uint64(i), "item", float64(i)*1.5)
		if err != nil {
			t.Fatalf("Append failed at row %d: %v", i, err)
		}
	}

	// Send batch
	if err := batch.Send(); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify count
	row := client.QueryRow(ctx, "SELECT count(*) FROM test_batch")
	var count uint64
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Count query failed: %v", err)
	}

	if count != 100 {
		t.Errorf("Count = %d, want 100", count)
	}
}

func TestIntegration_TypeMapping(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create table with various types
	createSQL := `
		CREATE TABLE IF NOT EXISTS test_types (
			col_int8 Int8,
			col_int64 Int64,
			col_uint64 UInt64,
			col_float32 Float32,
			col_float64 Float64,
			col_string String,
			col_uuid UUID,
			col_date Date,
			col_datetime DateTime,
			col_bool Bool,
			col_array Array(String),
			col_nullable Nullable(String)
		) ENGINE = MergeTree()
		ORDER BY col_int64
	`

	err := client.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Create table failed: %v", err)
	}
	defer client.Exec(ctx, "DROP TABLE IF EXISTS test_types")

	// Get columns and verify types
	columns, err := client.GetColumns(ctx, "test_types")
	if err != nil {
		t.Fatalf("GetColumns failed: %v", err)
	}

	expectedTypes := map[string]string{
		"col_int8":     "Int8",
		"col_int64":    "Int64",
		"col_uint64":   "UInt64",
		"col_float32":  "Float32",
		"col_float64":  "Float64",
		"col_string":   "String",
		"col_uuid":     "UUID",
		"col_date":     "Date",
		"col_datetime": "DateTime",
		"col_bool":     "Bool",
		"col_array":    "Array(String)",
		"col_nullable": "Nullable(String)",
	}

	for _, col := range columns {
		expected, ok := expectedTypes[col.Name]
		if !ok {
			t.Errorf("Unexpected column: %s", col.Name)
			continue
		}
		if col.Type != expected {
			t.Errorf("Column %s type = %q, want %q", col.Name, col.Type, expected)
		}
	}
}
