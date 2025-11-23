package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Telemetry holds all telemetry providers
type Telemetry struct {
	config   *Config
	tracer   *TracerProvider
	metrics  *MetricsProvider
	dbTracer *ClickHouseQueryTracer

	mu       sync.RWMutex
	shutdown bool
}

// New creates a new Telemetry instance from configuration
func New(ctx context.Context, cfg *Config) (*Telemetry, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid telemetry config: %w", err)
	}

	t := &Telemetry{
		config: cfg,
	}

	if !cfg.Enabled {
		return t, nil
	}

	// Initialize tracing
	if cfg.Tracing.Enabled {
		tracer, err := NewTracerProvider(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create tracer provider: %w", err)
		}
		t.tracer = tracer
		SetGlobalTracerProvider(tracer)
	}

	// Initialize metrics
	if cfg.Metrics.Enabled {
		metrics, err := NewMetricsProvider(ctx, cfg)
		if err != nil {
			// Clean up tracer if metrics fail
			if t.tracer != nil {
				t.tracer.Shutdown(ctx)
			}
			return nil, fmt.Errorf("failed to create metrics provider: %w", err)
		}
		t.metrics = metrics
		SetGlobalMetricsProvider(metrics)
	}

	// Initialize database tracer
	if cfg.Tracing.Enabled && cfg.Tracing.TraceDB {
		t.dbTracer = NewClickHouseQueryTracer("default", true)
		SetGlobalDBTracer(t.dbTracer)
	}

	return t, nil
}

// NewFromFile creates a Telemetry instance from a configuration file
func NewFromFile(ctx context.Context, configPath string) (*Telemetry, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return New(ctx, cfg)
}

// LoadConfig loads telemetry configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read telemetry config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse telemetry config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves telemetry configuration to a file
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write telemetry config: %w", err)
	}

	return nil
}

// Config returns the telemetry configuration
func (t *Telemetry) Config() *Config {
	return t.config
}

// Tracer returns the tracer provider
func (t *Telemetry) Tracer() *TracerProvider {
	return t.tracer
}

// Metrics returns the metrics provider
func (t *Telemetry) Metrics() *MetricsProvider {
	return t.metrics
}

// DBTracer returns the database tracer
func (t *Telemetry) DBTracer() *ClickHouseQueryTracer {
	return t.dbTracer
}

// SetDatabaseName updates the database name for the DB tracer
func (t *Telemetry) SetDatabaseName(name string) {
	if t.dbTracer != nil {
		t.dbTracer.dbName = name
	}
}

// IsEnabled returns whether telemetry is enabled
func (t *Telemetry) IsEnabled() bool {
	return t.config != nil && t.config.Enabled
}

// IsTracingEnabled returns whether tracing is enabled
func (t *Telemetry) IsTracingEnabled() bool {
	return t.IsEnabled() && t.config.Tracing.Enabled
}

// IsMetricsEnabled returns whether metrics are enabled
func (t *Telemetry) IsMetricsEnabled() bool {
	return t.IsEnabled() && t.config.Metrics.Enabled
}

// Shutdown shuts down all telemetry providers
func (t *Telemetry) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.shutdown {
		return nil
	}
	t.shutdown = true

	var errs []error

	if t.tracer != nil {
		if err := t.tracer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
		}
	}

	if t.metrics != nil {
		if err := t.metrics.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("metrics shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("telemetry shutdown errors: %v", errs)
	}

	return nil
}

// Global telemetry instance
var globalTelemetry *Telemetry
var globalMu sync.RWMutex

// Init initializes the global telemetry instance
func Init(ctx context.Context, cfg *Config) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	t, err := New(ctx, cfg)
	if err != nil {
		return err
	}

	globalTelemetry = t
	return nil
}

// InitFromFile initializes the global telemetry from a config file
func InitFromFile(ctx context.Context, configPath string) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	t, err := NewFromFile(ctx, configPath)
	if err != nil {
		return err
	}

	globalTelemetry = t
	return nil
}

// Get returns the global telemetry instance
func Get() *Telemetry {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalTelemetry
}

// Shutdown shuts down the global telemetry instance
func Shutdown(ctx context.Context) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalTelemetry != nil {
		return globalTelemetry.Shutdown(ctx)
	}
	return nil
}

// Example configuration for documentation
func ExampleConfig() *Config {
	return &Config{
		Enabled:        true,
		ServiceName:    "ndc-clickhouse",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		ResourceAttributes: map[string]string{
			"deployment.region": "us-east-1",
			"team":              "data-platform",
		},
		Tracing: TracingConfig{
			Enabled:  true,
			Exporter: "otlp",
			OTLP: OTLPExporterConfig{
				Endpoint: "otel-collector:4317",
				Protocol: "grpc",
				Headers: map[string]string{
					"Authorization": "Bearer ${OTEL_TOKEN}",
				},
				TLS: TLSConfig{
					Enabled: true,
				},
			},
			Sampling: SamplingConfig{
				Strategy: "parent_based",
				Ratio:    0.1, // Sample 10% of traces
				ParentBased: ParentBasedConfig{
					Root: "trace_id_ratio",
				},
			},
			SpanProcessor: SpanProcessorConfig{
				Type: "batch",
				Batch: BatchSpanProcessorConfig{
					MaxQueueSize:       4096,
					MaxExportBatchSize: 512,
				},
			},
			Propagators:    []string{"tracecontext", "baggage"},
			TraceDB:        true,
			TraceHTTP:      true,
			TraceGraphQL:   true,
			TraceFunctions: false,
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Exporter: "prometheus",
			Prometheus: PrometheusExporterConfig{
				Path:      "/metrics",
				Port:      9090,
				Namespace: "ndc_clickhouse",
			},
			HistogramBuckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
			},
			MetricPrefix:           "ndc_clickhouse_",
			CollectDBMetrics:       true,
			CollectHTTPMetrics:     true,
			CollectRuntimeMetrics:  true,
			CollectCacheMetrics:    true,
			CollectRateLimitMetrics: true,
		},
		Logging: LoggingConfig{
			Enabled:             true,
			Exporter:            "otlp",
			Level:               "info",
			IncludeTraceContext: true,
		},
	}
}
