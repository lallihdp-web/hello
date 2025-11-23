package analytics

import (
	"testing"
	"time"
)

func TestNewQueryLogger(t *testing.T) {
	config := DefaultLoggerConfig()
	logger, err := NewQueryLogger(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestQueryLogger_Log(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:       true,
		MaxInMemory:   100,
		SlowThreshold: 100 * time.Millisecond,
	})

	logger.Log(QueryLog{
		ID:         "test-1",
		SQL:        "SELECT * FROM users",
		Collection: "users",
		Operation:  "query",
		Duration:   50 * time.Millisecond,
		RowCount:   10,
	})

	stats := logger.GetStats()
	if stats.TotalQueries != 1 {
		t.Errorf("expected 1 query, got %d", stats.TotalQueries)
	}
}

func TestQueryLogger_SlowQuery(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:       true,
		MaxInMemory:   100,
		SlowThreshold: 100 * time.Millisecond,
	})

	// Fast query
	logger.Log(QueryLog{
		ID:       "fast-1",
		SQL:      "SELECT 1",
		Duration: 10 * time.Millisecond,
	})

	// Slow query
	logger.Log(QueryLog{
		ID:       "slow-1",
		SQL:      "SELECT * FROM big_table",
		Duration: 200 * time.Millisecond,
	})

	stats := logger.GetStats()
	if stats.SlowQueries != 1 {
		t.Errorf("expected 1 slow query, got %d", stats.SlowQueries)
	}
}

func TestQueryLogger_ErrorTracking(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 100,
	})

	logger.Log(QueryLog{
		ID:    "error-1",
		SQL:   "SELECT * FROM nonexistent",
		Error: "table not found",
	})

	stats := logger.GetStats()
	if stats.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", stats.TotalErrors)
	}
}

func TestQueryLogger_CollectionStats(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 100,
	})

	// Log queries for different collections
	logger.Log(QueryLog{
		ID:         "q1",
		SQL:        "SELECT * FROM users",
		Collection: "users",
		Duration:   50 * time.Millisecond,
		RowCount:   10,
	})
	logger.Log(QueryLog{
		ID:         "q2",
		SQL:        "SELECT * FROM users",
		Collection: "users",
		Duration:   60 * time.Millisecond,
		RowCount:   20,
	})
	logger.Log(QueryLog{
		ID:         "q3",
		SQL:        "SELECT * FROM orders",
		Collection: "orders",
		Duration:   100 * time.Millisecond,
		RowCount:   5,
	})

	stats := logger.GetStats()
	if stats.TotalQueries != 3 {
		t.Errorf("expected 3 queries, got %d", stats.TotalQueries)
	}

	userStats, ok := stats.CollectionStats["users"]
	if !ok {
		t.Fatal("expected users collection stats")
	}
	if userStats.QueryCount != 2 {
		t.Errorf("expected 2 user queries, got %d", userStats.QueryCount)
	}
}

func TestQueryLogger_OperationStats(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 100,
	})

	logger.Log(QueryLog{
		ID:        "q1",
		SQL:       "SELECT * FROM users",
		Operation: "query",
		Duration:  50 * time.Millisecond,
	})
	logger.Log(QueryLog{
		ID:        "q2",
		SQL:       "INSERT INTO users",
		Operation: "mutation",
		Duration:  30 * time.Millisecond,
	})

	stats := logger.GetStats()

	queryStats, ok := stats.OperationStats["query"]
	if !ok {
		t.Fatal("expected query operation stats")
	}
	if queryStats.Count != 1 {
		t.Errorf("expected 1 query operation, got %d", queryStats.Count)
	}

	mutationStats, ok := stats.OperationStats["mutation"]
	if !ok {
		t.Fatal("expected mutation operation stats")
	}
	if mutationStats.Count != 1 {
		t.Errorf("expected 1 mutation operation, got %d", mutationStats.Count)
	}
}

func TestQueryLogger_RecentQueries(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 3,
	})

	for i := 0; i < 5; i++ {
		logger.Log(QueryLog{
			ID:  string(rune('a' + i)),
			SQL: "SELECT " + string(rune('a'+i)),
		})
	}

	recent := logger.GetRecentQueries(10)
	if len(recent) > 3 {
		t.Errorf("expected max 3 recent queries, got %d", len(recent))
	}
}

func TestQueryLogger_GetErrorQueries(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 100,
	})

	logger.Log(QueryLog{
		ID:    "e1",
		SQL:   "SELECT * FROM bad",
		Error: "error 1",
	})
	logger.Log(QueryLog{
		ID:    "e2",
		SQL:   "SELECT * FROM worse",
		Error: "error 2",
	})
	logger.Log(QueryLog{
		ID:  "ok",
		SQL: "SELECT 1",
	})

	errors := logger.GetErrorQueries(10)
	if len(errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errors))
	}
}

func TestQueryLogger_GetSlowQueries(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:       true,
		MaxInMemory:   100,
		SlowThreshold: 50 * time.Millisecond,
	})

	logger.Log(QueryLog{
		ID:       "fast",
		SQL:      "SELECT 1",
		Duration: 10 * time.Millisecond,
	})
	logger.Log(QueryLog{
		ID:       "slow1",
		SQL:      "SELECT * FROM big",
		Duration: 100 * time.Millisecond,
	})
	logger.Log(QueryLog{
		ID:       "slow2",
		SQL:      "SELECT * FROM bigger",
		Duration: 200 * time.Millisecond,
	})

	slow := logger.GetSlowQueries(10)
	if len(slow) != 2 {
		t.Errorf("expected 2 slow queries, got %d", len(slow))
	}
}

func TestQueryLogger_Clear(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 100,
	})

	for i := 0; i < 5; i++ {
		logger.Log(QueryLog{
			ID:  string(rune('a' + i)),
			SQL: "SELECT " + string(rune('a'+i)),
		})
	}

	logger.Clear()

	stats := logger.GetStats()
	if stats.TotalQueries != 0 {
		t.Errorf("expected 0 queries after clear, got %d", stats.TotalQueries)
	}
}

func TestQueryLogger_Disabled(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     false,
		MaxInMemory: 100,
	})

	logger.Log(QueryLog{
		ID:  "q1",
		SQL: "SELECT 1",
	})

	stats := logger.GetStats()
	if stats.TotalQueries != 0 {
		t.Errorf("expected 0 queries when disabled, got %d", stats.TotalQueries)
	}
}

func TestQueryLogger_StartQuery(t *testing.T) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 100,
	})

	ctx := logger.StartQuery(nil, "users", "query")
	ctx.WithRole("admin").WithUserID("user-123")

	time.Sleep(10 * time.Millisecond)
	ctx.End(5, nil)

	stats := logger.GetStats()
	if stats.TotalQueries != 1 {
		t.Errorf("expected 1 query, got %d", stats.TotalQueries)
	}
}

// Benchmarks
func BenchmarkQueryLogger_Log(b *testing.B) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:       true,
		MaxInMemory:   10000,
		SlowThreshold: 100 * time.Millisecond,
	})

	log := QueryLog{
		ID:         "bench",
		SQL:        "SELECT * FROM users WHERE id = ?",
		Collection: "users",
		Operation:  "query",
		Duration:   50 * time.Millisecond,
		RowCount:   10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log(log)
	}
}

func BenchmarkQueryLogger_GetStats(b *testing.B) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 10000,
	})

	// Pre-populate
	for i := 0; i < 1000; i++ {
		logger.Log(QueryLog{
			ID:         string(rune(i)),
			SQL:        "SELECT " + string(rune(i)),
			Collection: string(rune(i % 10)),
			Operation:  "query",
			Duration:   time.Duration(i) * time.Millisecond,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.GetStats()
	}
}

func BenchmarkQueryLogger_Concurrent(b *testing.B) {
	logger, _ := NewQueryLogger(&LoggerConfig{
		Enabled:     true,
		MaxInMemory: 10000,
	})

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				logger.Log(QueryLog{
					ID:  string(rune(i)),
					SQL: "SELECT " + string(rune(i)),
				})
			} else {
				logger.GetStats()
			}
			i++
		}
	})
}
