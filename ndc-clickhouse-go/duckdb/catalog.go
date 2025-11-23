package duckdb

import (
	"sync"
	"time"
)

// DataCatalog tracks what data is cached in DuckDB vs ClickHouse
type DataCatalog struct {
	entries map[string]*CatalogEntry
	mu      sync.RWMutex
}

// CatalogEntry represents metadata about a cached collection
type CatalogEntry struct {
	Collection    string        `json:"collection"`
	RowCount      int64         `json:"row_count"`
	LastUpdated   time.Time     `json:"last_updated"`
	MaxAge        time.Duration `json:"max_age"`
	InDuckDB      bool          `json:"in_duckdb"`
	InClickHouse  bool          `json:"in_clickhouse"`
	SchemaHash    string        `json:"schema_hash,omitempty"`
	PartitionKeys []string      `json:"partition_keys,omitempty"`
	IndexColumns  []string      `json:"index_columns,omitempty"`
	SizeBytes     int64         `json:"size_bytes,omitempty"`
}

// NewDataCatalog creates a new data catalog
func NewDataCatalog() *DataCatalog {
	return &DataCatalog{
		entries: make(map[string]*CatalogEntry),
	}
}

// GetCollectionInfo returns information about a collection
func (c *DataCatalog) GetCollectionInfo(collection string) *CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[collection]
}

// SetCollectionInfo sets information about a collection
func (c *DataCatalog) SetCollectionInfo(collection string, entry *CatalogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[collection] = entry
}

// RemoveCollection removes a collection from the catalog
func (c *DataCatalog) RemoveCollection(collection string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, collection)
}

// Clear removes all entries from the catalog
func (c *DataCatalog) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CatalogEntry)
}

// ListCollections returns all tracked collections
func (c *DataCatalog) ListCollections() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	collections := make([]string, 0, len(c.entries))
	for col := range c.entries {
		collections = append(collections, col)
	}
	return collections
}

// GetAllEntries returns all catalog entries
func (c *DataCatalog) GetAllEntries() map[string]*CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make(map[string]*CatalogEntry, len(c.entries))
	for k, v := range c.entries {
		entries[k] = v
	}
	return entries
}

// IsFresh checks if a collection's cached data is fresh
func (c *DataCatalog) IsFresh(collection string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[collection]
	if !ok || !entry.InDuckDB {
		return false
	}
	return time.Since(entry.LastUpdated) <= entry.MaxAge
}

// GetStaleCollections returns collections that need refresh
func (c *DataCatalog) GetStaleCollections() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var stale []string
	for col, entry := range c.entries {
		if entry.InDuckDB && time.Since(entry.LastUpdated) > entry.MaxAge {
			stale = append(stale, col)
		}
	}
	return stale
}

// GetDuckDBCollections returns collections cached in DuckDB
func (c *DataCatalog) GetDuckDBCollections() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var collections []string
	for col, entry := range c.entries {
		if entry.InDuckDB {
			collections = append(collections, col)
		}
	}
	return collections
}

// Summary returns a summary of the catalog
func (c *DataCatalog) Summary() CatalogSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	summary := CatalogSummary{
		TotalCollections:  len(c.entries),
		DuckDBCollections: 0,
		FreshCollections:  0,
		StaleCollections:  0,
		TotalRows:         0,
		TotalSizeBytes:    0,
	}

	for _, entry := range c.entries {
		if entry.InDuckDB {
			summary.DuckDBCollections++
			summary.TotalRows += entry.RowCount
			summary.TotalSizeBytes += entry.SizeBytes

			if time.Since(entry.LastUpdated) <= entry.MaxAge {
				summary.FreshCollections++
			} else {
				summary.StaleCollections++
			}
		}
	}

	return summary
}

// CatalogSummary provides a summary of catalog state
type CatalogSummary struct {
	TotalCollections  int   `json:"total_collections"`
	DuckDBCollections int   `json:"duckdb_collections"`
	FreshCollections  int   `json:"fresh_collections"`
	StaleCollections  int   `json:"stale_collections"`
	TotalRows         int64 `json:"total_rows"`
	TotalSizeBytes    int64 `json:"total_size_bytes"`
}
