package duckdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "github.com/marcboeker/go-duckdb" // DuckDB driver
)

// Client provides DuckDB query caching functionality
type Client struct {
	db       *sql.DB
	config   *Config
	catalog  *DataCatalog
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewClient creates a new DuckDB client
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	db, err := sql.Open("duckdb", cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	// Set DuckDB configuration
	if _, err := db.Exec("SET memory_limit = ?", fmt.Sprintf("%dMB", cfg.MaxCacheSizeMB)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set memory limit: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		db:      db,
		config:  cfg,
		catalog: NewDataCatalog(),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Initialize schema
	if err := client.initSchema(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return client, nil
}

// initSchema creates the necessary tables for metadata tracking
func (c *Client) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS _cache_metadata (
			collection VARCHAR PRIMARY KEY,
			row_count BIGINT,
			last_updated TIMESTAMP,
			max_age_seconds BIGINT,
			schema_hash VARCHAR
		);

		CREATE TABLE IF NOT EXISTS _cache_stats (
			collection VARCHAR,
			query_hash VARCHAR,
			hit_count BIGINT DEFAULT 0,
			last_hit TIMESTAMP,
			avg_execution_ms DOUBLE,
			PRIMARY KEY (collection, query_hash)
		);
	`
	_, err := c.db.Exec(schema)
	return err
}

// CacheData stores data from ClickHouse into DuckDB
func (c *Client) CacheData(ctx context.Context, collection string, data []map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(data) == 0 {
		return nil
	}

	// Get collection config
	collConfig, hasConfig := c.config.Collections[collection]
	if hasConfig && !collConfig.Enabled {
		return nil
	}

	// Check row limit
	maxRows := c.config.Collections[collection].MaxRows
	if maxRows == 0 {
		maxRows = 1000000
	}
	if len(data) > maxRows {
		data = data[:maxRows]
	}

	// Create table from first row schema
	if err := c.createTableFromData(collection, data[0]); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Clear existing data
	if _, err := c.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", collection)); err != nil {
		return fmt.Errorf("failed to clear table: %w", err)
	}

	// Insert data in batches
	batchSize := 1000
	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}
		if err := c.insertBatch(ctx, collection, data[i:end]); err != nil {
			return fmt.Errorf("failed to insert batch: %w", err)
		}
	}

	// Update metadata
	maxAge := c.config.DefaultMaxAge
	if hasConfig && collConfig.MaxAge > 0 {
		maxAge = collConfig.MaxAge
	}

	_, err := c.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO _cache_metadata (collection, row_count, last_updated, max_age_seconds, schema_hash)
		VALUES (?, ?, ?, ?, ?)
	`, collection, len(data), time.Now(), int64(maxAge.Seconds()), "")

	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	// Update catalog
	c.catalog.SetCollectionInfo(collection, &CatalogEntry{
		Collection:  collection,
		RowCount:    int64(len(data)),
		LastUpdated: time.Now(),
		MaxAge:      maxAge,
		InDuckDB:    true,
	})

	return nil
}

// createTableFromData creates a DuckDB table based on the data schema
func (c *Client) createTableFromData(collection string, sample map[string]interface{}) error {
	columns := make([]string, 0, len(sample))
	for col, val := range sample {
		duckType := inferDuckDBType(val)
		columns = append(columns, fmt.Sprintf("%s %s", col, duckType))
	}

	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", collection, joinStrings(columns, ", "))
	_, err := c.db.Exec(sql)
	return err
}

// insertBatch inserts a batch of rows
func (c *Client) insertBatch(ctx context.Context, collection string, batch []map[string]interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	// Get column order from first row
	cols := make([]string, 0, len(batch[0]))
	for col := range batch[0] {
		cols = append(cols, col)
	}

	// Build insert statement
	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	placeholder := "(" + joinStrings(placeholders, ", ") + ")"

	values := make([]string, len(batch))
	args := make([]interface{}, 0, len(batch)*len(cols))

	for i, row := range batch {
		values[i] = placeholder
		for _, col := range cols {
			args = append(args, row[col])
		}
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		collection, joinStrings(cols, ", "), joinStrings(values, ", "))

	_, err := c.db.ExecContext(ctx, sql, args...)
	return err
}

// Query executes a query against cached DuckDB data
func (c *Client) Query(ctx context.Context, sql string, args ...interface{}) ([]map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := time.Now()

	rows, err := c.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	// Update stats
	duration := time.Since(start)
	c.updateQueryStats(ctx, sql, duration)

	return results, nil
}

// QueryCollection queries a specific collection with optional filters
func (c *Client) QueryCollection(ctx context.Context, collection string, filters map[string]interface{}, limit int) ([]map[string]interface{}, error) {
	// Check if data is fresh enough
	entry := c.catalog.GetCollectionInfo(collection)
	if entry == nil || !entry.InDuckDB {
		return nil, fmt.Errorf("collection %s not cached in DuckDB", collection)
	}

	if time.Since(entry.LastUpdated) > entry.MaxAge {
		return nil, fmt.Errorf("cached data for %s is stale", collection)
	}

	// Build query
	sql := fmt.Sprintf("SELECT * FROM %s", collection)
	args := make([]interface{}, 0)

	if len(filters) > 0 {
		conditions := make([]string, 0, len(filters))
		for col, val := range filters {
			conditions = append(conditions, fmt.Sprintf("%s = ?", col))
			args = append(args, val)
		}
		sql += " WHERE " + joinStrings(conditions, " AND ")
	}

	if limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", limit)
	}

	return c.Query(ctx, sql, args...)
}

// updateQueryStats updates query statistics
func (c *Client) updateQueryStats(ctx context.Context, sql string, duration time.Duration) {
	hash := hashString(sql)
	_, _ = c.db.ExecContext(ctx, `
		INSERT INTO _cache_stats (collection, query_hash, hit_count, last_hit, avg_execution_ms)
		VALUES ('_global', ?, 1, ?, ?)
		ON CONFLICT (collection, query_hash) DO UPDATE SET
			hit_count = hit_count + 1,
			last_hit = excluded.last_hit,
			avg_execution_ms = (avg_execution_ms * hit_count + excluded.avg_execution_ms) / (hit_count + 1)
	`, hash, time.Now(), float64(duration.Milliseconds()))
}

// IsCached checks if a collection is cached and fresh
func (c *Client) IsCached(collection string) bool {
	entry := c.catalog.GetCollectionInfo(collection)
	if entry == nil || !entry.InDuckDB {
		return false
	}
	return time.Since(entry.LastUpdated) <= entry.MaxAge
}

// GetCacheStats returns cache statistics
func (c *Client) GetCacheStats() (*CacheStats, error) {
	stats := &CacheStats{
		Collections: make(map[string]CollectionStats),
	}

	// Get collection stats
	rows, err := c.db.Query(`
		SELECT collection, row_count, last_updated, max_age_seconds
		FROM _cache_metadata
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var collection string
		var rowCount int64
		var lastUpdated time.Time
		var maxAgeSeconds int64

		if err := rows.Scan(&collection, &rowCount, &lastUpdated, &maxAgeSeconds); err != nil {
			continue
		}

		stats.Collections[collection] = CollectionStats{
			RowCount:    rowCount,
			LastUpdated: lastUpdated,
			MaxAge:      time.Duration(maxAgeSeconds) * time.Second,
			IsFresh:     time.Since(lastUpdated) <= time.Duration(maxAgeSeconds)*time.Second,
		}
		stats.TotalRows += rowCount
		stats.TotalCollections++
	}

	return stats, nil
}

// InvalidateCollection removes cached data for a collection
func (c *Client) InvalidateCollection(ctx context.Context, collection string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", collection)); err != nil {
		return err
	}

	if _, err := c.db.ExecContext(ctx, "DELETE FROM _cache_metadata WHERE collection = ?", collection); err != nil {
		return err
	}

	c.catalog.RemoveCollection(collection)
	return nil
}

// Clear removes all cached data
func (c *Client) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get all cached collections
	rows, err := c.db.QueryContext(ctx, "SELECT collection FROM _cache_metadata")
	if err != nil {
		return err
	}
	defer rows.Close()

	var collections []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err == nil {
			collections = append(collections, col)
		}
	}

	// Drop all tables
	for _, col := range collections {
		c.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", col))
	}

	// Clear metadata
	c.db.ExecContext(ctx, "DELETE FROM _cache_metadata")
	c.db.ExecContext(ctx, "DELETE FROM _cache_stats")

	c.catalog.Clear()
	return nil
}

// ExportToParquet exports a collection to Parquet format
func (c *Client) ExportToParquet(ctx context.Context, collection string, filePath string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	compression := c.config.ParquetCompression
	if compression == "" {
		compression = "snappy"
	}

	sql := fmt.Sprintf("COPY %s TO '%s' (FORMAT PARQUET, COMPRESSION '%s')",
		collection, filePath, compression)

	_, err := c.db.ExecContext(ctx, sql)
	return err
}

// ImportFromParquet imports data from a Parquet file
func (c *Client) ImportFromParquet(ctx context.Context, collection string, filePath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create table from Parquet schema
	sql := fmt.Sprintf("CREATE OR REPLACE TABLE %s AS SELECT * FROM read_parquet('%s')",
		collection, filePath)

	_, err := c.db.ExecContext(ctx, sql)
	if err != nil {
		return err
	}

	// Update metadata
	var rowCount int64
	row := c.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", collection))
	row.Scan(&rowCount)

	_, err = c.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO _cache_metadata (collection, row_count, last_updated, max_age_seconds, schema_hash)
		VALUES (?, ?, ?, ?, ?)
	`, collection, rowCount, time.Now(), int64(c.config.DefaultMaxAge.Seconds()), "")

	c.catalog.SetCollectionInfo(collection, &CatalogEntry{
		Collection:  collection,
		RowCount:    rowCount,
		LastUpdated: time.Now(),
		MaxAge:      c.config.DefaultMaxAge,
		InDuckDB:    true,
	})

	return err
}

// Close closes the DuckDB connection
func (c *Client) Close() error {
	c.cancel()
	return c.db.Close()
}

// CacheStats holds cache statistics
type CacheStats struct {
	TotalRows        int64                      `json:"total_rows"`
	TotalCollections int                        `json:"total_collections"`
	Collections      map[string]CollectionStats `json:"collections"`
}

// CollectionStats holds per-collection statistics
type CollectionStats struct {
	RowCount    int64         `json:"row_count"`
	LastUpdated time.Time     `json:"last_updated"`
	MaxAge      time.Duration `json:"max_age"`
	IsFresh     bool          `json:"is_fresh"`
}

// Helper functions

func inferDuckDBType(val interface{}) string {
	switch val.(type) {
	case int, int32, int64:
		return "BIGINT"
	case float32, float64:
		return "DOUBLE"
	case bool:
		return "BOOLEAN"
	case time.Time:
		return "TIMESTAMP"
	case []byte:
		return "BLOB"
	default:
		return "VARCHAR"
	}
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

func hashString(s string) string {
	data, _ := json.Marshal(s)
	return fmt.Sprintf("%x", data)[:16]
}
