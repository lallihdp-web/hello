package clickhouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/your-org/ndc-clickhouse-go/config"
)

// MultiClient manages connections to multiple ClickHouse databases
type MultiClient struct {
	clients  map[string]*Client
	primary  string
	mu       sync.RWMutex
	configs  map[string]*config.ConnectionConfig
}

// MultiClientConfig holds configuration for multiple databases
type MultiClientConfig struct {
	// Primary database name (used for default queries)
	Primary string `json:"primary"`

	// Database configurations
	Databases map[string]*config.ConnectionConfig `json:"databases"`
}

// NewMultiClient creates a new multi-database client
func NewMultiClient(cfg *MultiClientConfig) (*MultiClient, error) {
	mc := &MultiClient{
		clients: make(map[string]*Client),
		configs: cfg.Databases,
		primary: cfg.Primary,
	}

	// Connect to all databases
	for name, connCfg := range cfg.Databases {
		client, err := NewClient(connCfg)
		if err != nil {
			// Clean up already connected clients
			mc.Close()
			return nil, fmt.Errorf("failed to connect to database %s: %w", name, err)
		}
		mc.clients[name] = client
	}

	// Validate primary exists
	if mc.primary != "" {
		if _, exists := mc.clients[mc.primary]; !exists {
			mc.Close()
			return nil, fmt.Errorf("primary database %s not found", mc.primary)
		}
	}

	return mc, nil
}

// GetClient returns a client for the specified database
func (mc *MultiClient) GetClient(name string) (*Client, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if name == "" {
		name = mc.primary
	}

	client, exists := mc.clients[name]
	if !exists {
		return nil, fmt.Errorf("database %s not found", name)
	}

	return client, nil
}

// GetPrimary returns the primary database client
func (mc *MultiClient) GetPrimary() (*Client, error) {
	return mc.GetClient(mc.primary)
}

// ListDatabases returns the names of all connected databases
func (mc *MultiClient) ListDatabases() []string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	names := make([]string, 0, len(mc.clients))
	for name := range mc.clients {
		names = append(names, name)
	}
	return names
}

// AddDatabase adds a new database connection
func (mc *MultiClient) AddDatabase(name string, cfg *config.ConnectionConfig) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if _, exists := mc.clients[name]; exists {
		return fmt.Errorf("database %s already exists", name)
	}

	client, err := NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	mc.clients[name] = client
	mc.configs[name] = cfg

	return nil
}

// RemoveDatabase removes a database connection
func (mc *MultiClient) RemoveDatabase(name string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if name == mc.primary {
		return fmt.Errorf("cannot remove primary database")
	}

	client, exists := mc.clients[name]
	if !exists {
		return fmt.Errorf("database %s not found", name)
	}

	client.Close()
	delete(mc.clients, name)
	delete(mc.configs, name)

	return nil
}

// Close closes all database connections immediately
func (mc *MultiClient) Close() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for _, client := range mc.clients {
		client.Close()
	}
	mc.clients = make(map[string]*Client)
}

// GracefulClose initiates graceful shutdown for all database connections
func (mc *MultiClient) GracefulClose(ctx context.Context) error {
	mc.mu.Lock()
	clients := make(map[string]*Client)
	for name, client := range mc.clients {
		clients[name] = client
	}
	mc.mu.Unlock()

	var wg sync.WaitGroup
	var lastErr error
	var errMu sync.Mutex

	for name, client := range clients {
		wg.Add(1)
		go func(name string, client *Client) {
			defer wg.Done()
			if err := client.GracefulClose(ctx); err != nil {
				errMu.Lock()
				lastErr = fmt.Errorf("failed to close %s: %w", name, err)
				errMu.Unlock()
			}
		}(name, client)
	}

	wg.Wait()

	mc.mu.Lock()
	mc.clients = make(map[string]*Client)
	mc.mu.Unlock()

	return lastErr
}

// Shutdown gracefully shuts down all connections with timeout
func (mc *MultiClient) Shutdown(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return mc.GracefulClose(ctx)
}

// InFlightCount returns total in-flight requests across all clients
func (mc *MultiClient) InFlightCount() int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var total int64
	for _, client := range mc.clients {
		total += client.InFlightCount()
	}
	return total
}

// GetAllStats returns statistics for all clients
func (mc *MultiClient) GetAllStats() map[string]ClientStats {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	stats := make(map[string]ClientStats)
	for name, client := range mc.clients {
		stats[name] = client.GetStats()
	}
	return stats
}

// HealthCheck checks the health of all database connections
func (mc *MultiClient) HealthCheck(ctx context.Context) map[string]error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	results := make(map[string]error)

	for name, client := range mc.clients {
		_, err := client.Query(ctx, "SELECT 1")
		results[name] = err
	}

	return results
}

// QueryAll executes a query on all databases and returns combined results
func (mc *MultiClient) QueryAll(ctx context.Context, query string) (map[string][]map[string]interface{}, error) {
	mc.mu.RLock()
	clients := make(map[string]*Client)
	for name, client := range mc.clients {
		clients[name] = client
	}
	mc.mu.RUnlock()

	results := make(map[string][]map[string]interface{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var lastErr error

	for name, client := range clients {
		wg.Add(1)
		go func(name string, client *Client) {
			defer wg.Done()

			rows, err := client.Query(ctx, query)
			if err != nil {
				mu.Lock()
				lastErr = err
				mu.Unlock()
				return
			}
			defer rows.Close()

			columnTypes := rows.ColumnTypes()
			columns := make([]string, len(columnTypes))
			for i, ct := range columnTypes {
				columns[i] = ct.Name()
			}

			var dbResults []map[string]interface{}
			for rows.Next() {
				values := make([]interface{}, len(columns))
				valuePtrs := make([]interface{}, len(columns))
				for i := range values {
					valuePtrs[i] = &values[i]
				}

				if err := rows.Scan(valuePtrs...); err != nil {
					continue
				}

				row := make(map[string]interface{})
				for i, col := range columns {
					row[col] = values[i]
				}
				dbResults = append(dbResults, row)
			}

			mu.Lock()
			results[name] = dbResults
			mu.Unlock()
		}(name, client)
	}

	wg.Wait()

	if lastErr != nil && len(results) == 0 {
		return nil, lastErr
	}

	return results, nil
}

// GetAllTables returns tables from all databases
func (mc *MultiClient) GetAllTables(ctx context.Context) (map[string][]TableInfo, error) {
	mc.mu.RLock()
	clients := make(map[string]*Client)
	for name, client := range mc.clients {
		clients[name] = client
	}
	mc.mu.RUnlock()

	results := make(map[string][]TableInfo)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, client := range clients {
		wg.Add(1)
		go func(name string, client *Client) {
			defer wg.Done()

			tables, err := client.GetTables(ctx)
			if err != nil {
				return
			}

			mu.Lock()
			results[name] = tables
			mu.Unlock()
		}(name, client)
	}

	wg.Wait()
	return results, nil
}

// DatabaseInfo provides information about a database connection
type DatabaseInfo struct {
	Name      string `json:"name"`
	Database  string `json:"database"`
	IsPrimary bool   `json:"is_primary"`
	Connected bool   `json:"connected"`
}

// GetDatabaseInfo returns information about all databases
func (mc *MultiClient) GetDatabaseInfo(ctx context.Context) []DatabaseInfo {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	health := mc.HealthCheck(ctx)

	var infos []DatabaseInfo
	for name, client := range mc.clients {
		infos = append(infos, DatabaseInfo{
			Name:      name,
			Database:  client.Database(),
			IsPrimary: name == mc.primary,
			Connected: health[name] == nil,
		})
	}

	return infos
}
