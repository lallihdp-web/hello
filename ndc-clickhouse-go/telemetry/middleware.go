package telemetry

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddlewareConfig holds configuration for the HTTP middleware
type HTTPMiddlewareConfig struct {
	// Service name for spans
	ServiceName string

	// Skip tracing for certain paths (e.g., health checks)
	SkipPaths []string

	// Whether to record request/response bodies (can be expensive)
	RecordRequestBody  bool
	RecordResponseBody bool

	// Extract user info from headers
	UserIDHeader string
	RoleHeader   string

	// Custom attribute extractor
	AttributeExtractor func(r *http.Request) []attribute.KeyValue
}

// DefaultHTTPMiddlewareConfig returns default middleware configuration
func DefaultHTTPMiddlewareConfig() *HTTPMiddlewareConfig {
	return &HTTPMiddlewareConfig{
		ServiceName:  "ndc-clickhouse",
		SkipPaths:    []string{"/health", "/healthz", "/ready", "/metrics"},
		UserIDHeader: "X-Hasura-User-Id",
		RoleHeader:   "X-Hasura-Role",
	}
}

// HTTPMiddleware creates an HTTP middleware for tracing and metrics
func HTTPMiddleware(cfg *HTTPMiddlewareConfig) func(http.Handler) http.Handler {
	if cfg == nil {
		cfg = DefaultHTTPMiddlewareConfig()
	}

	tracer := otel.Tracer(cfg.ServiceName)
	propagator := otel.GetTextMapPropagator()

	skipPathsMap := make(map[string]bool)
	for _, path := range cfg.SkipPaths {
		skipPathsMap[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip certain paths
			if skipPathsMap[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Extract trace context from incoming request
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start span
			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPMethod(r.Method),
					semconv.HTTPURL(r.URL.String()),
					semconv.HTTPScheme(r.URL.Scheme),
					semconv.NetHostName(r.Host),
					semconv.HTTPUserAgent(r.UserAgent()),
					attribute.String("http.remote_addr", r.RemoteAddr),
				),
			)
			defer span.End()

			// Add user context if available
			if userID := r.Header.Get(cfg.UserIDHeader); userID != "" {
				span.SetAttributes(attribute.String("user.id", userID))
			}
			if role := r.Header.Get(cfg.RoleHeader); role != "" {
				span.SetAttributes(attribute.String("user.role", role))
			}

			// Add custom attributes
			if cfg.AttributeExtractor != nil {
				span.SetAttributes(cfg.AttributeExtractor(r)...)
			}

			// Wrap response writer to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Serve request with traced context
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record final span attributes
			duration := time.Since(start)
			span.SetAttributes(
				semconv.HTTPStatusCode(wrapped.statusCode),
				attribute.Int64("http.response.size", int64(wrapped.bytesWritten)),
			)

			// Record error if status code indicates failure
			if wrapped.statusCode >= 400 {
				span.SetAttributes(attribute.Bool("error", true))
			}

			// Record metrics
			if globalMetricsProvider != nil {
				globalMetricsProvider.RecordHTTPRequest(ctx, r.Method, r.URL.Path, wrapped.statusCode, duration)
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture response details
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Flush implements http.Flusher
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// GraphQLMiddleware creates middleware specifically for GraphQL endpoints
func GraphQLMiddleware(serviceName string) func(http.Handler) http.Handler {
	tracer := otel.Tracer(serviceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Extract operation info from request if possible
			// This is a simplified version - full implementation would parse GraphQL
			operationType := "query"
			if r.Method == "POST" {
				// Could parse body here to get operation name/type
				operationType = "operation"
			}

			ctx, span := tracer.Start(r.Context(), "graphql."+operationType,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("graphql.operation.type", operationType),
				),
			)
			defer span.End()

			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			duration := time.Since(start)
			span.SetAttributes(
				semconv.HTTPStatusCode(wrapped.statusCode),
				attribute.Float64("graphql.duration_ms", float64(duration.Milliseconds())),
			)

			if wrapped.statusCode >= 400 {
				span.SetAttributes(attribute.Bool("error", true))
			}
		})
	}
}

// TracingMiddleware is a simpler middleware that only adds tracing
func TracingMiddleware(tracer trace.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			span.SetAttributes(
				semconv.HTTPMethod(r.Method),
				semconv.HTTPURL(r.URL.String()),
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InjectTraceContext injects trace context into outgoing requests
func InjectTraceContext(ctx context.Context, r *http.Request) {
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))
}

// ExtractTraceContext extracts trace context from incoming requests
func ExtractTraceContext(r *http.Request) context.Context {
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
}

// TraceHTTPClient wraps an http.Client to add tracing
type TraceHTTPClient struct {
	client      *http.Client
	tracer      trace.Tracer
	serviceName string
}

// NewTraceHTTPClient creates a new traced HTTP client
func NewTraceHTTPClient(client *http.Client, serviceName string) *TraceHTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &TraceHTTPClient{
		client:      client,
		tracer:      otel.Tracer(serviceName),
		serviceName: serviceName,
	}
}

// Do executes a traced HTTP request
func (c *TraceHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	ctx, span := c.tracer.Start(ctx, "HTTP "+req.Method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPMethod(req.Method),
			semconv.HTTPURL(req.URL.String()),
		),
	)
	defer span.End()

	// Inject trace context into outgoing request
	InjectTraceContext(ctx, req)

	// Execute request
	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.Bool("error", true))
		return nil, err
	}

	span.SetAttributes(semconv.HTTPStatusCode(resp.StatusCode))

	if resp.StatusCode >= 400 {
		span.SetAttributes(attribute.Bool("error", true))
	}

	return resp, nil
}

// Get performs a traced GET request
func (c *TraceHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// ResponseWriterWithContext is a response writer that includes tracing context
type ResponseWriterWithContext struct {
	http.ResponseWriter
	ctx context.Context
}

// Context returns the request context
func (rw *ResponseWriterWithContext) Context() context.Context {
	return rw.ctx
}

// WrapResponseWriter wraps a response writer with context
func WrapResponseWriter(w http.ResponseWriter, ctx context.Context) *ResponseWriterWithContext {
	return &ResponseWriterWithContext{
		ResponseWriter: w,
		ctx:            ctx,
	}
}
