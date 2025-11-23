package clickhouse

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/your-org/ndc-clickhouse-go/config"
)

// ErrClientClosed is returned when operations are attempted on a closed client
var ErrClientClosed = errors.New("client is closed")

// ErrShuttingDown is returned when client is shutting down
var ErrShuttingDown = errors.New("client is shutting down")

// ClientState represents the current state of the client
type ClientState int32

const (
	StateActive ClientState = iota
	StateShuttingDown
	StateClosed
)

func (s ClientState) String() string {
	switch s {
	case StateActive:
		return "active"
	case StateShuttingDown:
		return "shutting_down"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ClientStats holds client statistics
type ClientStats struct {
	TotalQueries      int64
	SuccessfulQueries int64
	FailedQueries     int64
	TotalExecs        int64
	ActiveQueries     int64
}

// Client wraps the ClickHouse connection with graceful shutdown support
type Client struct {
	conn       driver.Conn
	database   string
	config     *config.ConnectionConfig
	state      atomic.Int32
	inFlight   atomic.Int64
	mu         sync.RWMutex
	closedChan chan struct{}
	closeOnce  sync.Once

	// Statistics
	totalQueries      atomic.Int64
	successfulQueries atomic.Int64
	failedQueries     atomic.Int64
	totalExecs        atomic.Int64
}

// NewClient creates a new ClickHouse client
func NewClient(cfg *config.ConnectionConfig) (*Client, error) {
	opts, err := parseConnectionOptions(cfg)
	if err != nil {
		return nil, err
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	client := &Client{
		conn:       conn,
		database:   cfg.Database,
		config:     cfg,
		closedChan: make(chan struct{}),
	}
	client.state.Store(int32(StateActive))

	return client, nil
}

// parseConnectionOptions converts config to clickhouse options
func parseConnectionOptions(cfg *config.ConnectionConfig) (*clickhouse.Options, error) {
	// Parse the URL
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection URL: %w", err)
	}

	host := u.Host
	if host == "" {
		host = "localhost:9000"
	}

	opts := &clickhouse.Options{
		Addr: []string{host},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Debug: false,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: time.Hour,
	}

	if opts.MaxOpenConns == 0 {
		opts.MaxOpenConns = 10
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = 5
	}

	// TLS configuration
	if cfg.Secure || u.Scheme == "clickhouses" || u.Scheme == "https" {
		opts.TLS = &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
	}

	return opts, nil
}

// Close closes the connection immediately
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.state.Store(int32(StateClosed))
		close(c.closedChan)
		err = c.conn.Close()
	})
	return err
}

// GracefulClose initiates graceful shutdown with timeout for in-flight requests
func (c *Client) GracefulClose(ctx context.Context) error {
	// Mark as shutting down
	if !c.state.CompareAndSwap(int32(StateActive), int32(StateShuttingDown)) {
		// Already shutting down or closed
		return nil
	}

	// Wait for in-flight requests to complete
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Timeout - force close
			return c.Close()
		case <-ticker.C:
			if c.inFlight.Load() == 0 {
				return c.Close()
			}
		}
	}
}

// Shutdown is an alias for GracefulClose with a default timeout
func (c *Client) Shutdown(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.GracefulClose(ctx)
}

// State returns the current client state
func (c *Client) State() ClientState {
	return ClientState(c.state.Load())
}

// IsActive returns true if the client is active and accepting requests
func (c *Client) IsActive() bool {
	return c.State() == StateActive
}

// InFlightCount returns the number of in-flight requests
func (c *Client) InFlightCount() int64 {
	return c.inFlight.Load()
}

// Done returns a channel that is closed when the client is closed
func (c *Client) Done() <-chan struct{} {
	return c.closedChan
}

// GetStats returns client statistics
func (c *Client) GetStats() ClientStats {
	return ClientStats{
		TotalQueries:      c.totalQueries.Load(),
		SuccessfulQueries: c.successfulQueries.Load(),
		FailedQueries:     c.failedQueries.Load(),
		TotalExecs:        c.totalExecs.Load(),
		ActiveQueries:     c.inFlight.Load(),
	}
}

// checkState verifies the client is ready for operations
func (c *Client) checkState() error {
	state := c.State()
	switch state {
	case StateClosed:
		return ErrClientClosed
	case StateShuttingDown:
		return ErrShuttingDown
	default:
		return nil
	}
}

// trackRequest tracks an in-flight request and returns a release function
func (c *Client) trackRequest() func(success bool) {
	c.inFlight.Add(1)
	c.totalQueries.Add(1)
	return func(success bool) {
		c.inFlight.Add(-1)
		if success {
			c.successfulQueries.Add(1)
		} else {
			c.failedQueries.Add(1)
		}
	}
}

// Database returns the current database name
func (c *Client) Database() string {
	return c.database
}

// Query executes a query and returns rows
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	if err := c.checkState(); err != nil {
		return nil, err
	}

	release := c.trackRequest()
	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		release(false)
		return nil, err
	}

	// Wrap rows to track when they're closed
	return &trackedRows{
		Rows:    rows,
		release: release,
	}, nil
}

// QueryRow executes a query expecting a single row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	// Note: QueryRow doesn't return error for connection issues,
	// errors are deferred to Scan
	c.inFlight.Add(1)
	c.totalQueries.Add(1)
	defer func() {
		c.inFlight.Add(-1)
		c.successfulQueries.Add(1)
	}()

	return c.conn.QueryRow(ctx, query, args...)
}

// Exec executes a query without returning rows
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	if err := c.checkState(); err != nil {
		return err
	}

	c.inFlight.Add(1)
	c.totalExecs.Add(1)
	defer c.inFlight.Add(-1)

	return c.conn.Exec(ctx, query, args...)
}

// PrepareBatch prepares a batch insert
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	if err := c.checkState(); err != nil {
		return nil, err
	}

	c.inFlight.Add(1)
	batch, err := c.conn.PrepareBatch(ctx, query)
	if err != nil {
		c.inFlight.Add(-1)
		return nil, err
	}

	// Wrap batch to track when it's sent/aborted
	return &trackedBatch{
		Batch:   batch,
		release: func() { c.inFlight.Add(-1) },
	}, nil
}

// trackedRows wraps driver.Rows to track request completion
type trackedRows struct {
	driver.Rows
	release  func(bool)
	released bool
}

func (r *trackedRows) Close() error {
	if !r.released {
		r.released = true
		r.release(true)
	}
	return r.Rows.Close()
}

// trackedBatch wraps driver.Batch to track request completion
type trackedBatch struct {
	driver.Batch
	release  func()
	released bool
}

func (b *trackedBatch) Send() error {
	if !b.released {
		b.released = true
		defer b.release()
	}
	return b.Batch.Send()
}

func (b *trackedBatch) Abort() error {
	if !b.released {
		b.released = true
		defer b.release()
	}
	return b.Batch.Abort()
}

// TableInfo holds metadata about a ClickHouse table
type TableInfo struct {
	Name        string
	Database    string
	Engine      string
	Comment     string
	TotalRows   uint64
	TotalBytes  uint64
	PartCount   uint64
}

// ColumnInfo holds metadata about a column
type ColumnInfo struct {
	Name              string
	Type              string
	DefaultKind       string
	DefaultExpression string
	Comment           string
	IsPrimaryKey      bool
	Position          uint64
}

// GetTables retrieves all tables in the database
func (c *Client) GetTables(ctx context.Context) ([]TableInfo, error) {
	query := `
		SELECT
			name,
			database,
			engine,
			comment,
			total_rows,
			total_bytes,
			partition_count
		FROM system.tables
		WHERE database = ?
		AND engine NOT IN ('View', 'MaterializedView', 'Dictionary')
		ORDER BY name
	`

	rows, err := c.Query(ctx, query, c.database)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		var totalRows, totalBytes, partCount *uint64

		if err := rows.Scan(&t.Name, &t.Database, &t.Engine, &t.Comment, &totalRows, &totalBytes, &partCount); err != nil {
			return nil, fmt.Errorf("failed to scan table row: %w", err)
		}

		if totalRows != nil {
			t.TotalRows = *totalRows
		}
		if totalBytes != nil {
			t.TotalBytes = *totalBytes
		}
		if partCount != nil {
			t.PartCount = *partCount
		}

		tables = append(tables, t)
	}

	return tables, nil
}

// GetColumns retrieves all columns for a table
func (c *Client) GetColumns(ctx context.Context, tableName string) ([]ColumnInfo, error) {
	query := `
		SELECT
			name,
			type,
			default_kind,
			default_expression,
			comment,
			is_in_primary_key,
			position
		FROM system.columns
		WHERE database = ?
		AND table = ?
		ORDER BY position
	`

	rows, err := c.Query(ctx, query, c.database, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var isPK uint8

		if err := rows.Scan(
			&col.Name,
			&col.Type,
			&col.DefaultKind,
			&col.DefaultExpression,
			&col.Comment,
			&isPK,
			&col.Position,
		); err != nil {
			return nil, fmt.Errorf("failed to scan column row: %w", err)
		}

		col.IsPrimaryKey = isPK == 1
		columns = append(columns, col)
	}

	return columns, nil
}

// GetAllColumns retrieves all columns for all tables in the database
func (c *Client) GetAllColumns(ctx context.Context) (map[string][]ColumnInfo, error) {
	query := `
		SELECT
			table,
			name,
			type,
			default_kind,
			default_expression,
			comment,
			is_in_primary_key,
			position
		FROM system.columns
		WHERE database = ?
		ORDER BY table, position
	`

	rows, err := c.Query(ctx, query, c.database)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]ColumnInfo)
	for rows.Next() {
		var tableName string
		var col ColumnInfo
		var isPK uint8

		if err := rows.Scan(
			&tableName,
			&col.Name,
			&col.Type,
			&col.DefaultKind,
			&col.DefaultExpression,
			&col.Comment,
			&isPK,
			&col.Position,
		); err != nil {
			return nil, fmt.Errorf("failed to scan column row: %w", err)
		}

		col.IsPrimaryKey = isPK == 1
		result[tableName] = append(result[tableName], col)
	}

	return result, nil
}
