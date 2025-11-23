package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// TracerProvider wraps the OpenTelemetry tracer provider
type TracerProvider struct {
	provider *sdktrace.TracerProvider
	config   *TracingConfig
	tracer   trace.Tracer
}

// NewTracerProvider creates a new tracer provider based on configuration
func NewTracerProvider(ctx context.Context, cfg *Config) (*TracerProvider, error) {
	if !cfg.Enabled || !cfg.Tracing.Enabled {
		return &TracerProvider{
			tracer: otel.Tracer(cfg.ServiceName),
		}, nil
	}

	// Create resource
	res, err := createResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter
	exporter, err := createTraceExporter(ctx, &cfg.Tracing)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create sampler
	sampler := createSampler(&cfg.Tracing.Sampling)

	// Create span processor
	var spanProcessor sdktrace.SpanProcessor
	if cfg.Tracing.SpanProcessor.Type == "simple" {
		spanProcessor = sdktrace.NewSimpleSpanProcessor(exporter)
	} else {
		batchOpts := []sdktrace.BatchSpanProcessorOption{}
		batch := cfg.Tracing.SpanProcessor.Batch
		if batch.MaxQueueSize > 0 {
			batchOpts = append(batchOpts, sdktrace.WithMaxQueueSize(batch.MaxQueueSize))
		}
		if batch.MaxExportBatchSize > 0 {
			batchOpts = append(batchOpts, sdktrace.WithMaxExportBatchSize(batch.MaxExportBatchSize))
		}
		if batch.ExportTimeout > 0 {
			batchOpts = append(batchOpts, sdktrace.WithExportTimeout(batch.ExportTimeout))
		}
		if batch.ScheduleDelay > 0 {
			batchOpts = append(batchOpts, sdktrace.WithBatchTimeout(batch.ScheduleDelay))
		}
		spanProcessor = sdktrace.NewBatchSpanProcessor(exporter, batchOpts...)
	}

	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithSpanProcessor(spanProcessor),
	)

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Set up propagators
	setupPropagators(cfg.Tracing.Propagators)

	return &TracerProvider{
		provider: provider,
		config:   &cfg.Tracing,
		tracer:   provider.Tracer(cfg.ServiceName),
	}, nil
}

// createResource creates an OpenTelemetry resource
func createResource(cfg *Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		attribute.String("environment", cfg.Environment),
	}

	// Add custom resource attributes
	for k, v := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.NewWithAttributes(
		semconv.SchemaURL,
		attrs...,
	), nil
}

// createTraceExporter creates the appropriate trace exporter
func createTraceExporter(ctx context.Context, cfg *TracingConfig) (sdktrace.SpanExporter, error) {
	switch cfg.Exporter {
	case "otlp":
		return createOTLPTraceExporter(ctx, &cfg.OTLP)
	case "zipkin":
		return createZipkinExporter(&cfg.Zipkin)
	case "stdout":
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	case "none":
		return &noopExporter{}, nil
	default:
		return createOTLPTraceExporter(ctx, &cfg.OTLP)
	}
}

// createOTLPTraceExporter creates an OTLP trace exporter
func createOTLPTraceExporter(ctx context.Context, cfg *OTLPExporterConfig) (*otlptrace.Exporter, error) {
	if cfg.Protocol == "http" {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.Endpoint),
		}

		if !cfg.TLS.Enabled {
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		if cfg.Compression == "gzip" {
			opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}

		if cfg.Timeout > 0 {
			opts = append(opts, otlptracehttp.WithTimeout(cfg.Timeout))
		}

		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
		}

		return otlptracehttp.New(ctx, opts...)
	}

	// Default to gRPC
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if !cfg.TLS.Enabled {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if cfg.Compression == "gzip" {
		opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
	}

	if cfg.Timeout > 0 {
		opts = append(opts, otlptracegrpc.WithTimeout(cfg.Timeout))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createZipkinExporter creates a Zipkin exporter
func createZipkinExporter(cfg *ZipkinExporterConfig) (*zipkin.Exporter, error) {
	return zipkin.New(cfg.Endpoint)
}

// createSampler creates the appropriate sampler
func createSampler(cfg *SamplingConfig) sdktrace.Sampler {
	switch cfg.Strategy {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "trace_id_ratio":
		return sdktrace.TraceIDRatioBased(cfg.Ratio)
	case "parent_based":
		rootSampler := getRootSampler(cfg.ParentBased.Root, cfg.Ratio)
		return sdktrace.ParentBased(rootSampler)
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

// getRootSampler returns the root sampler for parent-based sampling
func getRootSampler(strategy string, ratio float64) sdktrace.Sampler {
	switch strategy {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "trace_id_ratio":
		return sdktrace.TraceIDRatioBased(ratio)
	default:
		return sdktrace.AlwaysSample()
	}
}

// setupPropagators configures context propagators
func setupPropagators(propagators []string) {
	var props []propagation.TextMapPropagator

	for _, p := range propagators {
		switch p {
		case "tracecontext":
			props = append(props, propagation.TraceContext{})
		case "baggage":
			props = append(props, propagation.Baggage{})
		}
	}

	if len(props) == 0 {
		props = append(props, propagation.TraceContext{}, propagation.Baggage{})
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(props...))
}

// Tracer returns the tracer instance
func (tp *TracerProvider) Tracer() trace.Tracer {
	return tp.tracer
}

// Shutdown shuts down the tracer provider
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span
func (tp *TracerProvider) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tp.tracer.Start(ctx, name, opts...)
}

// noopExporter is a no-op span exporter
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}

// Span attribute helpers

// DBAttributes returns common database span attributes
func DBAttributes(system, name, statement string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.DBSystemKey.String(system),
		semconv.DBName(name),
		semconv.DBStatement(statement),
	}
}

// HTTPAttributes returns common HTTP span attributes
func HTTPAttributes(method, url string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPMethod(method),
		semconv.HTTPURL(url),
		semconv.HTTPStatusCode(statusCode),
	}
}

// GraphQLAttributes returns GraphQL operation attributes
func GraphQLAttributes(operationType, operationName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("graphql.operation.type", operationType),
		attribute.String("graphql.operation.name", operationName),
	}
}

// ErrorAttributes returns error span attributes
func ErrorAttributes(err error) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Bool("error", true),
		attribute.String("error.message", err.Error()),
	}
}

// Global tracer accessor
var globalTracerProvider *TracerProvider

// SetGlobalTracerProvider sets the global tracer provider
func SetGlobalTracerProvider(tp *TracerProvider) {
	globalTracerProvider = tp
}

// GetTracer returns the global tracer
func GetTracer() trace.Tracer {
	if globalTracerProvider != nil {
		return globalTracerProvider.Tracer()
	}
	return otel.Tracer("ndc-clickhouse")
}

// StartSpanFromContext starts a span from the global tracer
func StartSpanFromContext(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name, opts...)
}

// SpanFromContext returns the span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// WithTraceID returns a context with the trace ID for logging
func WithTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// WithSpanID returns the span ID for logging
func WithSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// TraceContextFromEnv extracts trace context from environment variables
func TraceContextFromEnv() context.Context {
	ctx := context.Background()

	// Check for W3C trace context in environment
	traceParent := os.Getenv("TRACEPARENT")
	traceState := os.Getenv("TRACESTATE")

	if traceParent != "" {
		carrier := propagation.MapCarrier{
			"traceparent": traceParent,
		}
		if traceState != "" {
			carrier["tracestate"] = traceState
		}

		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, carrier)
	}

	return ctx
}
