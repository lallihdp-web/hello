package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsProvider wraps the OpenTelemetry meter provider
type MetricsProvider struct {
	provider       *sdkmetric.MeterProvider
	config         *MetricsConfig
	meter          metric.Meter
	promHandler    http.Handler
	promRegisterer promclient.Registerer

	// Pre-created instruments
	queryCounter        metric.Int64Counter
	queryDuration       metric.Float64Histogram
	queryRowsReturned   metric.Int64Counter
	queryErrorCounter   metric.Int64Counter
	cacheHitCounter     metric.Int64Counter
	cacheMissCounter    metric.Int64Counter
	rateLimitCounter    metric.Int64Counter
	httpRequestCounter  metric.Int64Counter
	httpRequestDuration metric.Float64Histogram
	activeConnections   metric.Int64UpDownCounter
}

// NewMetricsProvider creates a new metrics provider based on configuration
func NewMetricsProvider(ctx context.Context, cfg *Config) (*MetricsProvider, error) {
	mp := &MetricsProvider{
		config: &cfg.Metrics,
	}

	if !cfg.Enabled || !cfg.Metrics.Enabled {
		mp.meter = otel.Meter(cfg.ServiceName)
		mp.createInstruments(cfg.Metrics.MetricPrefix)
		return mp, nil
	}

	// Create resource
	res, err := createMetricsResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter and reader
	var reader sdkmetric.Reader

	switch cfg.Metrics.Exporter {
	case "prometheus":
		exporter, promHandler, err := createPrometheusExporter(&cfg.Metrics.Prometheus)
		if err != nil {
			return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
		}
		reader = exporter
		mp.promHandler = promHandler

	case "otlp":
		exporter, err := createOTLPMetricExporter(ctx, &cfg.Metrics.OTLP)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
		}
		interval := cfg.Metrics.ExportInterval
		if interval == 0 {
			interval = 10 * time.Second
		}
		reader = sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval))

	case "stdout":
		exporter, err := stdoutmetric.New(stdoutmetric.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout metric exporter: %w", err)
		}
		interval := cfg.Metrics.ExportInterval
		if interval == 0 {
			interval = 10 * time.Second
		}
		reader = sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval))

	default:
		// No-op reader
		reader = sdkmetric.NewManualReader()
	}

	// Create histogram view with custom buckets if specified
	var views []sdkmetric.View
	if len(cfg.Metrics.HistogramBuckets) > 0 {
		views = append(views, sdkmetric.NewView(
			sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: cfg.Metrics.HistogramBuckets,
				},
			},
		))
	}

	// Create meter provider
	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	}
	for _, view := range views {
		opts = append(opts, sdkmetric.WithView(view))
	}

	provider := sdkmetric.NewMeterProvider(opts...)

	// Set global meter provider
	otel.SetMeterProvider(provider)

	mp.provider = provider
	mp.meter = provider.Meter(cfg.ServiceName)

	// Create instruments
	mp.createInstruments(cfg.Metrics.MetricPrefix)

	// Start runtime metrics collection if enabled
	if cfg.Metrics.CollectRuntimeMetrics {
		go mp.collectRuntimeMetrics(ctx)
	}

	return mp, nil
}

// createMetricsResource creates an OpenTelemetry resource for metrics
func createMetricsResource(cfg *Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		attribute.String("environment", cfg.Environment),
	}

	for k, v := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.NewWithAttributes(
		semconv.SchemaURL,
		attrs...,
	), nil
}

// createPrometheusExporter creates a Prometheus exporter
func createPrometheusExporter(cfg *PrometheusExporterConfig) (sdkmetric.Reader, http.Handler, error) {
	registry := promclient.NewRegistry()

	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
		prometheus.WithNamespace(cfg.Namespace),
	)
	if err != nil {
		return nil, nil, err
	}

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	return exporter, handler, nil
}

// createOTLPMetricExporter creates an OTLP metric exporter
func createOTLPMetricExporter(ctx context.Context, cfg *OTLPExporterConfig) (sdkmetric.Exporter, error) {
	if cfg.Protocol == "http" {
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(cfg.Endpoint),
		}

		if !cfg.TLS.Enabled {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}

		if cfg.Compression == "gzip" {
			opts = append(opts, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
		}

		if cfg.Timeout > 0 {
			opts = append(opts, otlpmetrichttp.WithTimeout(cfg.Timeout))
		}

		if len(cfg.Headers) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(cfg.Headers))
		}

		return otlpmetrichttp.New(ctx, opts...)
	}

	// Default to gRPC
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
	}

	if !cfg.TLS.Enabled {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	if cfg.Compression == "gzip" {
		opts = append(opts, otlpmetricgrpc.WithCompressor("gzip"))
	}

	if cfg.Timeout > 0 {
		opts = append(opts, otlpmetricgrpc.WithTimeout(cfg.Timeout))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}

	return otlpmetricgrpc.New(ctx, opts...)
}

// createInstruments creates all metric instruments
func (mp *MetricsProvider) createInstruments(prefix string) {
	var err error

	// Query metrics
	mp.queryCounter, err = mp.meter.Int64Counter(
		prefix+"queries_total",
		metric.WithDescription("Total number of queries executed"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		mp.queryCounter, _ = mp.meter.Int64Counter("noop_counter")
	}

	mp.queryDuration, err = mp.meter.Float64Histogram(
		prefix+"query_duration_seconds",
		metric.WithDescription("Query execution duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		mp.queryDuration, _ = mp.meter.Float64Histogram("noop_histogram")
	}

	mp.queryRowsReturned, err = mp.meter.Int64Counter(
		prefix+"query_rows_total",
		metric.WithDescription("Total number of rows returned by queries"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		mp.queryRowsReturned, _ = mp.meter.Int64Counter("noop_counter")
	}

	mp.queryErrorCounter, err = mp.meter.Int64Counter(
		prefix+"query_errors_total",
		metric.WithDescription("Total number of query errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		mp.queryErrorCounter, _ = mp.meter.Int64Counter("noop_counter")
	}

	// Cache metrics
	mp.cacheHitCounter, err = mp.meter.Int64Counter(
		prefix+"cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
		metric.WithUnit("{hit}"),
	)
	if err != nil {
		mp.cacheHitCounter, _ = mp.meter.Int64Counter("noop_counter")
	}

	mp.cacheMissCounter, err = mp.meter.Int64Counter(
		prefix+"cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
		metric.WithUnit("{miss}"),
	)
	if err != nil {
		mp.cacheMissCounter, _ = mp.meter.Int64Counter("noop_counter")
	}

	// Rate limit metrics
	mp.rateLimitCounter, err = mp.meter.Int64Counter(
		prefix+"rate_limit_total",
		metric.WithDescription("Total number of rate limited requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		mp.rateLimitCounter, _ = mp.meter.Int64Counter("noop_counter")
	}

	// HTTP metrics
	mp.httpRequestCounter, err = mp.meter.Int64Counter(
		prefix+"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		mp.httpRequestCounter, _ = mp.meter.Int64Counter("noop_counter")
	}

	mp.httpRequestDuration, err = mp.meter.Float64Histogram(
		prefix+"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		mp.httpRequestDuration, _ = mp.meter.Float64Histogram("noop_histogram")
	}

	// Connection metrics
	mp.activeConnections, err = mp.meter.Int64UpDownCounter(
		prefix+"active_connections",
		metric.WithDescription("Number of active database connections"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		mp.activeConnections, _ = mp.meter.Int64UpDownCounter("noop_updown")
	}
}

// collectRuntimeMetrics periodically collects Go runtime metrics
func (mp *MetricsProvider) collectRuntimeMetrics(ctx context.Context) {
	prefix := mp.config.MetricPrefix
	if prefix == "" {
		prefix = "ndc_clickhouse_"
	}

	// Create runtime gauges
	goroutines, _ := mp.meter.Int64ObservableGauge(
		prefix+"runtime_goroutines",
		metric.WithDescription("Number of goroutines"),
	)

	heapAlloc, _ := mp.meter.Int64ObservableGauge(
		prefix+"runtime_heap_alloc_bytes",
		metric.WithDescription("Heap allocation in bytes"),
	)

	heapObjects, _ := mp.meter.Int64ObservableGauge(
		prefix+"runtime_heap_objects",
		metric.WithDescription("Number of allocated heap objects"),
	)

	gcPauseTotal, _ := mp.meter.Float64ObservableGauge(
		prefix+"runtime_gc_pause_total_seconds",
		metric.WithDescription("Total GC pause time in seconds"),
	)

	// Register callback
	_, _ = mp.meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		o.ObserveInt64(goroutines, int64(runtime.NumGoroutine()))
		o.ObserveInt64(heapAlloc, int64(memStats.HeapAlloc))
		o.ObserveInt64(heapObjects, int64(memStats.HeapObjects))
		o.ObserveFloat64(gcPauseTotal, float64(memStats.PauseTotalNs)/1e9)

		return nil
	}, goroutines, heapAlloc, heapObjects, gcPauseTotal)
}

// Meter returns the meter instance
func (mp *MetricsProvider) Meter() metric.Meter {
	return mp.meter
}

// PrometheusHandler returns the Prometheus HTTP handler
func (mp *MetricsProvider) PrometheusHandler() http.Handler {
	if mp.promHandler != nil {
		return mp.promHandler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Prometheus metrics not enabled"))
	})
}

// Shutdown shuts down the metrics provider
func (mp *MetricsProvider) Shutdown(ctx context.Context) error {
	if mp.provider != nil {
		return mp.provider.Shutdown(ctx)
	}
	return nil
}

// RecordQuery records a query execution
func (mp *MetricsProvider) RecordQuery(ctx context.Context, collection, operation string, duration time.Duration, rowCount int64, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("collection", collection),
		attribute.String("operation", operation),
	}

	mp.queryCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	mp.queryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if rowCount > 0 {
		mp.queryRowsReturned.Add(ctx, rowCount, metric.WithAttributes(attrs...))
	}

	if err != nil {
		errorAttrs := append(attrs, attribute.String("error_type", fmt.Sprintf("%T", err)))
		mp.queryErrorCounter.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
	}
}

// RecordCacheHit records a cache hit
func (mp *MetricsProvider) RecordCacheHit(ctx context.Context, cacheType string) {
	mp.cacheHitCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("cache_type", cacheType),
	))
}

// RecordCacheMiss records a cache miss
func (mp *MetricsProvider) RecordCacheMiss(ctx context.Context, cacheType string) {
	mp.cacheMissCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("cache_type", cacheType),
	))
}

// RecordRateLimit records a rate limit event
func (mp *MetricsProvider) RecordRateLimit(ctx context.Context, limiterType, role string) {
	mp.rateLimitCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("limiter_type", limiterType),
		attribute.String("role", role),
	))
}

// RecordHTTPRequest records an HTTP request
func (mp *MetricsProvider) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.Int("status_code", statusCode),
	}

	mp.httpRequestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	mp.httpRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// UpdateActiveConnections updates the active connection count
func (mp *MetricsProvider) UpdateActiveConnections(ctx context.Context, delta int64) {
	mp.activeConnections.Add(ctx, delta)
}

// Global metrics provider accessor
var globalMetricsProvider *MetricsProvider

// SetGlobalMetricsProvider sets the global metrics provider
func SetGlobalMetricsProvider(mp *MetricsProvider) {
	globalMetricsProvider = mp
}

// GetMetrics returns the global metrics provider
func GetMetrics() *MetricsProvider {
	return globalMetricsProvider
}
