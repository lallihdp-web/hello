package duckdb

import (
	"context"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	if client.db == nil {
		t.Error("expected non-nil database connection")
	}
}

func TestCacheData(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Test data
	data := []map[string]interface{}{
		{"id": int64(1), "name": "Alice", "age": int64(30)},
		{"id": int64(2), "name": "Bob", "age": int64(25)},
		{"id": int64(3), "name": "Charlie", "age": int64(35)},
	}

	// Cache the data
	err = client.CacheData(ctx, "users", data)
	if err != nil {
		t.Fatalf("failed to cache data: %v", err)
	}

	// Verify it's cached
	if !client.IsCached("users") {
		t.Error("expected users to be cached")
	}

	// Query the cached data
	results, err := client.Query(ctx, "SELECT * FROM users ORDER BY id")
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 rows, got %d", len(results))
	}
}

func TestQueryCollection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Cache test data
	data := []map[string]interface{}{
		{"id": int64(1), "name": "Alice", "department": "Engineering"},
		{"id": int64(2), "name": "Bob", "department": "Sales"},
		{"id": int64(3), "name": "Charlie", "department": "Engineering"},
	}

	err = client.CacheData(ctx, "employees", data)
	if err != nil {
		t.Fatalf("failed to cache data: %v", err)
	}

	// Query with filter
	results, err := client.QueryCollection(ctx, "employees", map[string]interface{}{
		"department": "Engineering",
	}, 10)

	if err != nil {
		t.Fatalf("failed to query collection: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 rows, got %d", len(results))
	}
}

func TestInvalidateCollection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Cache data
	data := []map[string]interface{}{
		{"id": int64(1), "name": "Alice"},
	}
	client.CacheData(ctx, "test_collection", data)

	if !client.IsCached("test_collection") {
		t.Error("expected collection to be cached")
	}

	// Invalidate
	err = client.InvalidateCollection(ctx, "test_collection")
	if err != nil {
		t.Fatalf("failed to invalidate: %v", err)
	}

	if client.IsCached("test_collection") {
		t.Error("expected collection to be invalidated")
	}
}

func TestClear(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Cache multiple collections
	data1 := []map[string]interface{}{{"id": int64(1)}}
	data2 := []map[string]interface{}{{"id": int64(2)}}

	client.CacheData(ctx, "coll1", data1)
	client.CacheData(ctx, "coll2", data2)

	// Clear all
	err = client.Clear(ctx)
	if err != nil {
		t.Fatalf("failed to clear: %v", err)
	}

	if client.IsCached("coll1") || client.IsCached("coll2") {
		t.Error("expected all collections to be cleared")
	}
}

func TestGetCacheStats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Cache data
	data := []map[string]interface{}{
		{"id": int64(1)},
		{"id": int64(2)},
		{"id": int64(3)},
	}
	client.CacheData(ctx, "stats_test", data)

	stats, err := client.GetCacheStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalCollections != 1 {
		t.Errorf("expected 1 collection, got %d", stats.TotalCollections)
	}

	if stats.TotalRows != 3 {
		t.Errorf("expected 3 rows, got %d", stats.TotalRows)
	}

	collStats, ok := stats.Collections["stats_test"]
	if !ok {
		t.Fatal("expected stats_test collection stats")
	}

	if collStats.RowCount != 3 {
		t.Errorf("expected 3 rows in collection, got %d", collStats.RowCount)
	}

	if !collStats.IsFresh {
		t.Error("expected data to be fresh")
	}
}

// Data Catalog Tests

func TestDataCatalog(t *testing.T) {
	catalog := NewDataCatalog()

	// Test set and get
	entry := &CatalogEntry{
		Collection:  "users",
		RowCount:    1000,
		LastUpdated: time.Now(),
		MaxAge:      time.Hour,
		InDuckDB:    true,
	}
	catalog.SetCollectionInfo("users", entry)

	retrieved := catalog.GetCollectionInfo("users")
	if retrieved == nil {
		t.Fatal("expected to get catalog entry")
	}

	if retrieved.RowCount != 1000 {
		t.Errorf("expected row count 1000, got %d", retrieved.RowCount)
	}

	// Test IsFresh
	if !catalog.IsFresh("users") {
		t.Error("expected collection to be fresh")
	}

	// Test remove
	catalog.RemoveCollection("users")
	if catalog.GetCollectionInfo("users") != nil {
		t.Error("expected collection to be removed")
	}
}

func TestCatalogSummary(t *testing.T) {
	catalog := NewDataCatalog()

	// Add entries
	catalog.SetCollectionInfo("fresh", &CatalogEntry{
		Collection:  "fresh",
		RowCount:    100,
		LastUpdated: time.Now(),
		MaxAge:      time.Hour,
		InDuckDB:    true,
	})

	catalog.SetCollectionInfo("stale", &CatalogEntry{
		Collection:  "stale",
		RowCount:    200,
		LastUpdated: time.Now().Add(-2 * time.Hour),
		MaxAge:      time.Hour,
		InDuckDB:    true,
	})

	summary := catalog.Summary()

	if summary.TotalCollections != 2 {
		t.Errorf("expected 2 collections, got %d", summary.TotalCollections)
	}

	if summary.DuckDBCollections != 2 {
		t.Errorf("expected 2 DuckDB collections, got %d", summary.DuckDBCollections)
	}

	if summary.FreshCollections != 1 {
		t.Errorf("expected 1 fresh collection, got %d", summary.FreshCollections)
	}

	if summary.StaleCollections != 1 {
		t.Errorf("expected 1 stale collection, got %d", summary.StaleCollections)
	}

	if summary.TotalRows != 300 {
		t.Errorf("expected 300 total rows, got %d", summary.TotalRows)
	}
}

// Hybrid Engine Tests

func TestQueryPlanToSQL(t *testing.T) {
	tests := []struct {
		name     string
		plan     *QueryPlan
		expected string
	}{
		{
			name: "simple select",
			plan: &QueryPlan{
				Collection: "users",
			},
			expected: "SELECT * FROM users",
		},
		{
			name: "select with columns",
			plan: &QueryPlan{
				Collection: "users",
				Columns:    []string{"id", "name"},
			},
			expected: "SELECT id, name FROM users",
		},
		{
			name: "select with filter",
			plan: &QueryPlan{
				Collection: "users",
				Columns:    []string{"*"},
				Filters:    map[string]interface{}{"department": "Engineering"},
			},
			expected: "SELECT * FROM users WHERE department = 'Engineering'",
		},
		{
			name: "select with limit",
			plan: &QueryPlan{
				Collection: "users",
				Limit:      10,
			},
			expected: "SELECT * FROM users LIMIT 10",
		},
		{
			name: "aggregation",
			plan: &QueryPlan{
				Collection: "orders",
				GroupBy:    []string{"department"},
				Aggregations: []Aggregation{
					{Function: "count", Column: "*", Alias: "total"},
					{Function: "sum", Column: "amount", Alias: "total_amount"},
				},
			},
			expected: "SELECT department, COUNT(*) AS total, SUM(amount) AS total_amount FROM orders GROUP BY department",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql := tt.plan.ToSQL()
			if sql != tt.expected {
				t.Errorf("expected:\n%s\ngot:\n%s", tt.expected, sql)
			}
		})
	}
}

func TestQueryRouterDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Disabled hybrid config
	hybridConfig := &HybridConfig{Enabled: false}
	router := NewQueryRouter(client, hybridConfig)

	ctx := context.Background()
	query := &QueryPlan{Collection: "users"}

	decision, err := router.RouteQuery(ctx, query)
	if err != nil {
		t.Fatalf("routing failed: %v", err)
	}

	if decision.Engine != EngineClickHouse {
		t.Errorf("expected ClickHouse when hybrid disabled, got %s", decision.Engine)
	}
}

func TestQueryRouterCachedData(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"
	cfg.Hybrid.Enabled = true
	cfg.Hybrid.PreferDuckDBUnderRows = 10000

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Cache some data
	data := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		data[i] = map[string]interface{}{"id": int64(i)}
	}
	client.CacheData(ctx, "small_table", data)

	router := NewQueryRouter(client, &cfg.Hybrid)
	query := &QueryPlan{Collection: "small_table"}

	decision, err := router.RouteQuery(ctx, query)
	if err != nil {
		t.Fatalf("routing failed: %v", err)
	}

	if decision.Engine != EngineDuckDB {
		t.Errorf("expected DuckDB for small cached table, got %s (reason: %s)", decision.Engine, decision.Reason)
	}
}

func TestCostModel(t *testing.T) {
	model := NewCostModel()

	entry := &CatalogEntry{
		RowCount: 10000,
	}

	query := &QueryPlan{
		Collection: "users",
	}

	clickhouseCost := model.EstimateClickHouseCost(query, entry)
	duckdbCost := model.EstimateDuckDBCost(query, entry)

	// DuckDB should be cheaper due to no network latency
	if duckdbCost >= clickhouseCost {
		t.Errorf("expected DuckDB cost (%.4f) to be less than ClickHouse cost (%.4f)",
			duckdbCost, clickhouseCost)
	}

	// Test with aggregation
	queryWithAgg := &QueryPlan{
		Collection: "users",
		Aggregations: []Aggregation{
			{Function: "count", Column: "*"},
		},
	}

	clickhouseCostAgg := model.EstimateClickHouseCost(queryWithAgg, entry)
	if clickhouseCostAgg <= clickhouseCost {
		t.Error("expected aggregation to increase cost")
	}
}

func TestRouterStats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	router := NewQueryRouter(client, &HybridConfig{Enabled: false})

	ctx := context.Background()

	// Route several queries
	for i := 0; i < 5; i++ {
		router.RouteQuery(ctx, &QueryPlan{Collection: "test"})
	}

	stats := router.GetStats()
	if stats.ClickHouseQueries != 5 {
		t.Errorf("expected 5 ClickHouse queries, got %d", stats.ClickHouseQueries)
	}

	// Reset stats
	router.ResetStats()
	stats = router.GetStats()
	if stats.ClickHouseQueries != 0 {
		t.Error("expected stats to be reset")
	}
}

// Benchmarks

func BenchmarkCacheData(b *testing.B) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, _ := NewClient(cfg)
	defer client.Close()

	ctx := context.Background()

	// Generate test data
	data := make([]map[string]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		data[i] = map[string]interface{}{
			"id":   int64(i),
			"name": "test",
			"age":  int64(i % 100),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.CacheData(ctx, "bench_table", data)
	}
}

func BenchmarkQuery(b *testing.B) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"

	client, _ := NewClient(cfg)
	defer client.Close()

	ctx := context.Background()

	// Cache data
	data := make([]map[string]interface{}, 10000)
	for i := 0; i < 10000; i++ {
		data[i] = map[string]interface{}{
			"id":   int64(i),
			"name": "test",
			"age":  int64(i % 100),
		}
	}
	client.CacheData(ctx, "bench_query", data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.Query(ctx, "SELECT * FROM bench_query WHERE age > 50 LIMIT 100")
	}
}

func BenchmarkRouteQuery(b *testing.B) {
	cfg := DefaultConfig()
	cfg.DatabasePath = ":memory:"
	cfg.Hybrid.Enabled = true

	client, _ := NewClient(cfg)
	defer client.Close()

	ctx := context.Background()

	// Cache some data
	data := make([]map[string]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		data[i] = map[string]interface{}{"id": int64(i)}
	}
	client.CacheData(ctx, "bench_route", data)

	router := NewQueryRouter(client, &cfg.Hybrid)
	query := &QueryPlan{Collection: "bench_route"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.RouteQuery(ctx, query)
	}
}
