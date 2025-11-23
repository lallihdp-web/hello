package telemetry

import (
	"time"
)

// Config holds all OpenTelemetry configuration
type Config struct {
	// Enable telemetry (master switch)
	Enabled bool `json:"enabled"`

	// Service identification
	ServiceName    string `json:"service_name"`
	ServiceVersion string `json:"service_version"`
	Environment    string `json:"environment"`

	// Resource attributes
	ResourceAttributes map[string]string `json:"resource_attributes,omitempty"`

	// Tracing configuration
	Tracing TracingConfig `json:"tracing"`

	// Metrics configuration
	Metrics MetricsConfig `json:"metrics"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`
}

// TracingConfig holds tracing-specific configuration
type TracingConfig struct {
	// Enable tracing
	Enabled bool `json:"enabled"`

	// Exporter type: "otlp", "jaeger", "zipkin", "stdout", "none"
	Exporter string `json:"exporter"`

	// OTLP exporter settings
	OTLP OTLPExporterConfig `json:"otlp,omitempty"`

	// Jaeger exporter settings
	Jaeger JaegerExporterConfig `json:"jaeger,omitempty"`

	// Zipkin exporter settings
	Zipkin ZipkinExporterConfig `json:"zipkin,omitempty"`

	// Sampling configuration
	Sampling SamplingConfig `json:"sampling"`

	// Span processing
	SpanProcessor SpanProcessorConfig `json:"span_processor"`

	// Propagation format: "tracecontext", "baggage", "b3", "jaeger", "all"
	Propagators []string `json:"propagators,omitempty"`

	// Trace specific operations
	TraceDB        bool `json:"trace_db"`
	TraceHTTP      bool `json:"trace_http"`
	TraceGraphQL   bool `json:"trace_graphql"`
	TraceFunctions bool `json:"trace_functions"`
}

// MetricsConfig holds metrics-specific configuration
type MetricsConfig struct {
	// Enable metrics
	Enabled bool `json:"enabled"`

	// Exporter type: "otlp", "prometheus", "stdout", "none"
	Exporter string `json:"exporter"`

	// OTLP exporter settings
	OTLP OTLPExporterConfig `json:"otlp,omitempty"`

	// Prometheus exporter settings
	Prometheus PrometheusExporterConfig `json:"prometheus,omitempty"`

	// Export interval for push-based exporters
	ExportInterval time.Duration `json:"export_interval,omitempty"`

	// Histogram bucket boundaries
	HistogramBuckets []float64 `json:"histogram_buckets,omitempty"`

	// Metric prefixes
	MetricPrefix string `json:"metric_prefix,omitempty"`

	// Which metrics to collect
	CollectDBMetrics       bool `json:"collect_db_metrics"`
	CollectHTTPMetrics     bool `json:"collect_http_metrics"`
	CollectRuntimeMetrics  bool `json:"collect_runtime_metrics"`
	CollectCacheMetrics    bool `json:"collect_cache_metrics"`
	CollectRateLimitMetrics bool `json:"collect_rate_limit_metrics"`
}

// LoggingConfig holds logging-specific configuration
type LoggingConfig struct {
	// Enable OpenTelemetry logging
	Enabled bool `json:"enabled"`

	// Exporter type: "otlp", "stdout", "none"
	Exporter string `json:"exporter"`

	// OTLP exporter settings
	OTLP OTLPExporterConfig `json:"otlp,omitempty"`

	// Log level: "debug", "info", "warn", "error"
	Level string `json:"level"`

	// Include trace context in logs
	IncludeTraceContext bool `json:"include_trace_context"`
}

// OTLPExporterConfig holds OTLP exporter settings
type OTLPExporterConfig struct {
	// Endpoint URL (e.g., "localhost:4317" for gRPC, "localhost:4318" for HTTP)
	Endpoint string `json:"endpoint"`

	// Protocol: "grpc" or "http"
	Protocol string `json:"protocol"`

	// Headers to include in requests
	Headers map[string]string `json:"headers,omitempty"`

	// TLS settings
	TLS TLSConfig `json:"tls,omitempty"`

	// Compression: "none", "gzip"
	Compression string `json:"compression,omitempty"`

	// Timeout for requests
	Timeout time.Duration `json:"timeout,omitempty"`

	// Retry settings
	Retry RetryConfig `json:"retry,omitempty"`
}

// JaegerExporterConfig holds Jaeger exporter settings
type JaegerExporterConfig struct {
	// Agent endpoint (UDP) for Jaeger agent
	AgentHost string `json:"agent_host,omitempty"`
	AgentPort int    `json:"agent_port,omitempty"`

	// Collector endpoint (HTTP) for Jaeger collector
	CollectorEndpoint string `json:"collector_endpoint,omitempty"`

	// Username and password for collector
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ZipkinExporterConfig holds Zipkin exporter settings
type ZipkinExporterConfig struct {
	// Endpoint URL
	Endpoint string `json:"endpoint"`
}

// PrometheusExporterConfig holds Prometheus exporter settings
type PrometheusExporterConfig struct {
	// HTTP endpoint path for metrics (default: "/metrics")
	Path string `json:"path,omitempty"`

	// Port to serve metrics on
	Port int `json:"port,omitempty"`

	// Namespace for metrics
	Namespace string `json:"namespace,omitempty"`
}

// SamplingConfig holds sampling configuration
type SamplingConfig struct {
	// Sampling strategy: "always_on", "always_off", "trace_id_ratio", "parent_based"
	Strategy string `json:"strategy"`

	// Ratio for trace_id_ratio sampler (0.0 to 1.0)
	Ratio float64 `json:"ratio,omitempty"`

	// Parent-based sampling configuration
	ParentBased ParentBasedConfig `json:"parent_based,omitempty"`
}

// ParentBasedConfig holds parent-based sampling configuration
type ParentBasedConfig struct {
	// Root sampler when no parent
	Root string `json:"root"`

	// Remote parent sampled
	RemoteParentSampled string `json:"remote_parent_sampled,omitempty"`

	// Remote parent not sampled
	RemoteParentNotSampled string `json:"remote_parent_not_sampled,omitempty"`

	// Local parent sampled
	LocalParentSampled string `json:"local_parent_sampled,omitempty"`

	// Local parent not sampled
	LocalParentNotSampled string `json:"local_parent_not_sampled,omitempty"`
}

// SpanProcessorConfig holds span processor configuration
type SpanProcessorConfig struct {
	// Type: "batch" or "simple"
	Type string `json:"type"`

	// Batch processor settings
	Batch BatchSpanProcessorConfig `json:"batch,omitempty"`
}

// BatchSpanProcessorConfig holds batch span processor settings
type BatchSpanProcessorConfig struct {
	// Maximum queue size
	MaxQueueSize int `json:"max_queue_size,omitempty"`

	// Maximum export batch size
	MaxExportBatchSize int `json:"max_export_batch_size,omitempty"`

	// Export timeout
	ExportTimeout time.Duration `json:"export_timeout,omitempty"`

	// Schedule delay between exports
	ScheduleDelay time.Duration `json:"schedule_delay,omitempty"`
}

// TLSConfig holds TLS settings
type TLSConfig struct {
	// Enable TLS
	Enabled bool `json:"enabled"`

	// Skip certificate verification (insecure)
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`

	// CA certificate file path
	CAFile string `json:"ca_file,omitempty"`

	// Client certificate file path
	CertFile string `json:"cert_file,omitempty"`

	// Client key file path
	KeyFile string `json:"key_file,omitempty"`
}

// RetryConfig holds retry settings
type RetryConfig struct {
	// Enable retries
	Enabled bool `json:"enabled"`

	// Initial backoff interval
	InitialInterval time.Duration `json:"initial_interval,omitempty"`

	// Maximum backoff interval
	MaxInterval time.Duration `json:"max_interval,omitempty"`

	// Maximum elapsed time
	MaxElapsedTime time.Duration `json:"max_elapsed_time,omitempty"`
}

// DefaultConfig returns the default telemetry configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:        false,
		ServiceName:    "ndc-clickhouse",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Tracing: TracingConfig{
			Enabled:  true,
			Exporter: "otlp",
			OTLP: OTLPExporterConfig{
				Endpoint: "localhost:4317",
				Protocol: "grpc",
				Timeout:  10 * time.Second,
			},
			Sampling: SamplingConfig{
				Strategy: "parent_based",
				Ratio:    1.0,
				ParentBased: ParentBasedConfig{
					Root: "always_on",
				},
			},
			SpanProcessor: SpanProcessorConfig{
				Type: "batch",
				Batch: BatchSpanProcessorConfig{
					MaxQueueSize:       2048,
					MaxExportBatchSize: 512,
					ExportTimeout:      30 * time.Second,
					ScheduleDelay:      5 * time.Second,
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
			ExportInterval: 10 * time.Second,
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
			Enabled:             false,
			Exporter:            "stdout",
			Level:               "info",
			IncludeTraceContext: true,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.ServiceName == "" {
		c.ServiceName = "ndc-clickhouse"
	}

	// Validate tracing config
	if c.Tracing.Enabled {
		switch c.Tracing.Exporter {
		case "otlp", "jaeger", "zipkin", "stdout", "none":
			// Valid
		default:
			c.Tracing.Exporter = "otlp"
		}

		switch c.Tracing.Sampling.Strategy {
		case "always_on", "always_off", "trace_id_ratio", "parent_based":
			// Valid
		default:
			c.Tracing.Sampling.Strategy = "parent_based"
		}
	}

	// Validate metrics config
	if c.Metrics.Enabled {
		switch c.Metrics.Exporter {
		case "otlp", "prometheus", "stdout", "none":
			// Valid
		default:
			c.Metrics.Exporter = "prometheus"
		}
	}

	return nil
}
