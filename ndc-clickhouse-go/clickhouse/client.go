package clickhouse

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/your-org/ndc-clickhouse-go/config"
)

// Client wraps the ClickHouse connection
type Client struct {
	conn     driver.Conn
	database string
	config   *config.ConnectionConfig
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

	return &Client{
		conn:     conn,
		database: cfg.Database,
		config:   cfg,
	}, nil
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

// Close closes the connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Database returns the current database name
func (c *Client) Database() string {
	return c.database
}

// Query executes a query and returns rows
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.conn.Query(ctx, query, args...)
}

// QueryRow executes a query expecting a single row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return c.conn.QueryRow(ctx, query, args...)
}

// Exec executes a query without returning rows
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.conn.Exec(ctx, query, args...)
}

// PrepareBatch prepares a batch insert
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	return c.conn.PrepareBatch(ctx, query)
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
