package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Configuration holds the connector configuration
type Configuration struct {
	// Connection settings
	Connection ConnectionConfig `json:"connection"`

	// Tables configuration (optional overrides)
	Tables map[string]TableConfig `json:"tables,omitempty"`

	// Native queries (raw SQL as virtual tables)
	NativeQueries map[string]NativeQuery `json:"native_queries,omitempty"`

	// Relationships between tables
	Relationships RelationshipsConfig `json:"relationships,omitempty"`

	// Permissions for row-level security
	Permissions PermissionsConfig `json:"permissions,omitempty"`

	// Metadata settings
	Metadata MetadataConfig `json:"metadata,omitempty"`

	// Telemetry settings (OpenTelemetry)
	Telemetry TelemetryConfig `json:"telemetry,omitempty"`
}

// TelemetryConfig holds OpenTelemetry configuration
type TelemetryConfig struct {
	// Enable telemetry
	Enabled bool `json:"enabled"`

	// Service name
	ServiceName string `json:"service_name,omitempty"`

	// Service version
	ServiceVersion string `json:"service_version,omitempty"`

	// Environment (e.g., "production", "staging")
	Environment string `json:"environment,omitempty"`

	// Tracing configuration
	Tracing TelemetryTracingConfig `json:"tracing,omitempty"`

	// Metrics configuration
	Metrics TelemetryMetricsConfig `json:"metrics,omitempty"`
}

// TelemetryTracingConfig holds tracing configuration
type TelemetryTracingConfig struct {
	// Enable tracing
	Enabled bool `json:"enabled"`

	// Exporter type: "otlp", "jaeger", "zipkin", "stdout", "none"
	Exporter string `json:"exporter,omitempty"`

	// OTLP endpoint (for OTLP exporter)
	Endpoint string `json:"endpoint,omitempty"`

	// Protocol: "grpc" or "http"
	Protocol string `json:"protocol,omitempty"`

	// Sampling ratio (0.0 to 1.0)
	SamplingRatio float64 `json:"sampling_ratio,omitempty"`

	// Sampling strategy: "always_on", "always_off", "trace_id_ratio", "parent_based"
	SamplingStrategy string `json:"sampling_strategy,omitempty"`

	// Trace database queries
	TraceDB bool `json:"trace_db,omitempty"`

	// Trace HTTP requests
	TraceHTTP bool `json:"trace_http,omitempty"`

	// Trace GraphQL operations
	TraceGraphQL bool `json:"trace_graphql,omitempty"`
}

// TelemetryMetricsConfig holds metrics configuration
type TelemetryMetricsConfig struct {
	// Enable metrics
	Enabled bool `json:"enabled"`

	// Exporter type: "otlp", "prometheus", "stdout", "none"
	Exporter string `json:"exporter,omitempty"`

	// OTLP endpoint (for OTLP exporter)
	Endpoint string `json:"endpoint,omitempty"`

	// Prometheus port (for prometheus exporter)
	PrometheusPort int `json:"prometheus_port,omitempty"`

	// Prometheus path (for prometheus exporter)
	PrometheusPath string `json:"prometheus_path,omitempty"`

	// Metric prefix
	MetricPrefix string `json:"metric_prefix,omitempty"`

	// Collect database metrics
	CollectDBMetrics bool `json:"collect_db_metrics,omitempty"`

	// Collect HTTP metrics
	CollectHTTPMetrics bool `json:"collect_http_metrics,omitempty"`

	// Collect Go runtime metrics
	CollectRuntimeMetrics bool `json:"collect_runtime_metrics,omitempty"`
}

// MetadataConfig holds metadata settings
type MetadataConfig struct {
	// Version of the configuration schema
	Version string `json:"version,omitempty"`

	// Description of this connector instance
	Description string `json:"description,omitempty"`

	// Tags for categorization
	Tags []string `json:"tags,omitempty"`
}

// ConnectionConfig holds ClickHouse connection parameters
type ConnectionConfig struct {
	// ClickHouse server URL (e.g., "clickhouse://localhost:9000")
	URL string `json:"url"`

	// Username for authentication
	Username string `json:"username,omitempty"`

	// Password for authentication
	Password string `json:"password,omitempty"`

	// Database name
	Database string `json:"database"`

	// Enable secure connection (TLS)
	Secure bool `json:"secure,omitempty"`

	// Skip TLS certificate verification
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`

	// Connection pool settings
	MaxOpenConns int `json:"max_open_conns,omitempty"`
	MaxIdleConns int `json:"max_idle_conns,omitempty"`
}

// TableConfig holds per-table configuration
type TableConfig struct {
	// Alias name to expose in GraphQL (optional)
	Alias string `json:"alias,omitempty"`

	// Whether to exclude this table from schema
	Exclude bool `json:"exclude,omitempty"`

	// Column configurations
	Columns map[string]ColumnConfig `json:"columns,omitempty"`

	// Primary key columns (for mutations)
	PrimaryKey []string `json:"primary_key,omitempty"`
}

// ColumnConfig holds per-column configuration
type ColumnConfig struct {
	// Alias name to expose in GraphQL
	Alias string `json:"alias,omitempty"`

	// Whether to exclude this column
	Exclude bool `json:"exclude,omitempty"`
}

// NativeQuery defines a raw SQL query exposed as a collection
type NativeQuery struct {
	// SQL query with parameter placeholders: {param_name:Type}
	SQL string `json:"sql"`

	// Return type - either inline columns or reference to a table
	Columns map[string]string `json:"columns,omitempty"`

	// Reference to an existing table's type
	ReturnType string `json:"return_type,omitempty"`

	// Arguments for the query
	Arguments map[string]ArgumentConfig `json:"arguments,omitempty"`

	// Description for GraphQL schema
	Description string `json:"description,omitempty"`
}

// ArgumentConfig defines a query argument
type ArgumentConfig struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// LoadConfiguration loads configuration from a directory
func LoadConfiguration(configDir string) (*Configuration, error) {
	configPath := filepath.Join(configDir, "configuration.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default configuration
			return &Configuration{
				Tables:        make(map[string]TableConfig),
				NativeQueries: make(map[string]NativeQuery),
			}, nil
		}
		return nil, fmt.Errorf("failed to read configuration: %w", err)
	}

	var config Configuration
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Apply environment variable substitution
	config.Connection.URL = expandEnv(config.Connection.URL)
	config.Connection.Username = expandEnv(config.Connection.Username)
	config.Connection.Password = expandEnv(config.Connection.Password)
	config.Connection.Database = expandEnv(config.Connection.Database)

	return &config, nil
}

// expandEnv expands environment variables in the format ${VAR} or $VAR
func expandEnv(s string) string {
	return os.ExpandEnv(s)
}

// SaveConfiguration saves configuration to a directory
func SaveConfiguration(configDir string, config *Configuration) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "configuration.json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	return nil
}
