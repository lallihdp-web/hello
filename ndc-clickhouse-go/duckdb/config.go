package duckdb

import (
	"time"
)

// Config holds DuckDB integration configuration
type Config struct {
	// Enable DuckDB integration
	Enabled bool `json:"enabled"`

	// Database file path (":memory:" for in-memory)
	DatabasePath string `json:"database_path,omitempty"`

	// Maximum cache size in MB
	MaxCacheSizeMB int `json:"max_cache_size_mb,omitempty"`

	// Default data freshness requirement
	DefaultMaxAge time.Duration `json:"default_max_age,omitempty"`

	// Export format: "parquet", "csv", or "json"
	ExportFormat string `json:"export_format,omitempty"`

	// Parquet compression: "snappy", "gzip", "zstd", "none"
	ParquetCompression string `json:"parquet_compression,omitempty"`

	// Collections to cache in DuckDB
	Collections map[string]CollectionConfig `json:"collections,omitempty"`

	// Hybrid engine settings
	Hybrid HybridConfig `json:"hybrid,omitempty"`
}

// CollectionConfig holds per-collection DuckDB settings
type CollectionConfig struct {
	// Enable caching for this collection
	Enabled bool `json:"enabled"`

	// Maximum age of cached data before refresh
	MaxAge time.Duration `json:"max_age,omitempty"`

	// Maximum rows to cache
	MaxRows int `json:"max_rows,omitempty"`

	// Refresh interval for automatic updates
	RefreshInterval time.Duration `json:"refresh_interval,omitempty"`

	// Partition columns for efficient queries
	PartitionBy []string `json:"partition_by,omitempty"`

	// Index columns for faster lookups
	IndexColumns []string `json:"index_columns,omitempty"`
}

// HybridConfig holds hybrid engine configuration
type HybridConfig struct {
	// Enable hybrid query routing
	Enabled bool `json:"enabled"`

	// Prefer DuckDB for queries under this row count
	PreferDuckDBUnderRows int `json:"prefer_duckdb_under_rows,omitempty"`

	// Cost threshold - use DuckDB if estimated cost is lower
	CostThreshold float64 `json:"cost_threshold,omitempty"`

	// Enable query analysis for routing decisions
	EnableQueryAnalysis bool `json:"enable_query_analysis,omitempty"`

	// Fallback to ClickHouse on DuckDB errors
	FallbackOnError bool `json:"fallback_on_error,omitempty"`
}

// DefaultConfig returns default DuckDB configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:            false,
		DatabasePath:       ":memory:",
		MaxCacheSizeMB:     1024, // 1GB
		DefaultMaxAge:      time.Hour,
		ExportFormat:       "parquet",
		ParquetCompression: "snappy",
		Collections:        make(map[string]CollectionConfig),
		Hybrid: HybridConfig{
			Enabled:               false,
			PreferDuckDBUnderRows: 100000,
			CostThreshold:         0.5,
			EnableQueryAnalysis:   true,
			FallbackOnError:       true,
		},
	}
}

// CollectionDefaultConfig returns default collection configuration
func CollectionDefaultConfig() CollectionConfig {
	return CollectionConfig{
		Enabled:         true,
		MaxAge:          time.Hour,
		MaxRows:         1000000,
		RefreshInterval: time.Minute * 30,
	}
}
