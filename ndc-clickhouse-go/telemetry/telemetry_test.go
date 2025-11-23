package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("expected non-nil default config")
	}

	if cfg.ServiceName != "ndc-clickhouse" {
		t.Errorf("expected service name 'ndc-clickhouse', got %s", cfg.ServiceName)
	}

	if !cfg.Tracing.Enabled {
		t.Error("expected tracing to be enabled by default")
	}

	if !cfg.Metrics.Enabled {
		t.Error("expected metrics to be enabled by default")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid disabled config",
			cfg: &Config{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid enabled config",
			cfg: &Config{
				Enabled:     true,
				ServiceName: "test-service",
				Tracing: TracingConfig{
					Enabled:  true,
					Exporter: "stdout",
				},
				Metrics: MetricsConfig{
					Enabled:  true,
					Exporter: "stdout",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid exporter gets defaulted",
			cfg: &Config{
				Enabled: true,
				Tracing: TracingConfig{
					Enabled:  true,
					Exporter: "invalid",
				},
			},
			wantErr: false, // Should default to otlp
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewTelemetry_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled: false,
	}

	tel, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tel.IsEnabled() {
		t.Error("expected telemetry to be disabled")
	}

	if tel.IsTracingEnabled() {
		t.Error("expected tracing to be disabled")
	}

	if tel.IsMetricsEnabled() {
		t.Error("expected metrics to be disabled")
	}

	// Shutdown should work even when disabled
	if err := tel.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
}

func TestNewTelemetry_WithStdout(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Tracing: TracingConfig{
			Enabled:  true,
			Exporter: "none", // Use none to avoid stdout noise in tests
			Sampling: SamplingConfig{
				Strategy: "always_on",
			},
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Exporter: "none",
		},
	}

	tel, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer tel.Shutdown(ctx)

	if !tel.IsEnabled() {
		t.Error("expected telemetry to be enabled")
	}

	if !tel.IsTracingEnabled() {
		t.Error("expected tracing to be enabled")
	}

	if !tel.IsMetricsEnabled() {
		t.Error("expected metrics to be enabled")
	}
}

func TestTracerProvider(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Tracing: TracingConfig{
			Enabled:  true,
			Exporter: "none",
			Sampling: SamplingConfig{
				Strategy: "always_on",
			},
		},
		Metrics: MetricsConfig{
			Enabled: false,
		},
	}

	tp, err := NewTracerProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer tp.Shutdown(ctx)

	// Test starting a span
	tracer := tp.Tracer()
	ctx, span := tracer.Start(ctx, "test-span")
	if span == nil {
		t.Error("expected non-nil span")
	}
	span.End()

	// Test helper functions
	ctx, span2 := tp.StartSpan(ctx, "test-span-2")
	if span2 == nil {
		t.Error("expected non-nil span from StartSpan")
	}
	span2.End()
}

func TestMetricsProvider(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Tracing: TracingConfig{
			Enabled: false,
		},
		Metrics: MetricsConfig{
			Enabled:      true,
			Exporter:     "none",
			MetricPrefix: "test_",
		},
	}

	mp, err := NewMetricsProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mp.Shutdown(ctx)

	// Test recording metrics
	mp.RecordQuery(ctx, "users", "SELECT", 100*time.Millisecond, 10, nil)
	mp.RecordCacheHit(ctx, "query")
	mp.RecordCacheMiss(ctx, "query")
	mp.RecordRateLimit(ctx, "token_bucket", "anonymous")
	mp.RecordHTTPRequest(ctx, "GET", "/api/query", 200, 50*time.Millisecond)
	mp.UpdateActiveConnections(ctx, 1)
	mp.UpdateActiveConnections(ctx, -1)
}

func TestHTTPMiddleware(t *testing.T) {
	cfg := DefaultHTTPMiddlewareConfig()
	middleware := HTTPMiddleware(cfg)

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with middleware
	wrapped := middleware(handler)

	// Test request
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Hasura-User-Id", "user-123")
	req.Header.Set("X-Hasura-Role", "admin")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHTTPMiddleware_SkipPaths(t *testing.T) {
	cfg := &HTTPMiddlewareConfig{
		SkipPaths: []string{"/health"},
	}
	middleware := HTTPMiddleware(cfg)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware(handler)

	// Test skipped path
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for skipped path, got %d", w.Code)
	}
}

func TestDBTracer(t *testing.T) {
	tracer := NewClickHouseQueryTracer("testdb", true)

	ctx := context.Background()

	// Test SELECT tracing
	ctx, finish := tracer.TraceSelect(ctx, "users", "SELECT * FROM users LIMIT 10")
	finish(10, nil)

	// Test INSERT tracing
	ctx, finish = tracer.TraceInsert(ctx, "users", "INSERT INTO users VALUES (?)")
	finish(1, nil)

	// Test with error
	ctx, finish = tracer.TraceSelect(ctx, "users", "SELECT * FROM nonexistent")
	finish(0, context.DeadlineExceeded)
}

func TestDBTracer_SQLTruncation(t *testing.T) {
	tracer := NewDBTracer(&DBTracerConfig{
		ServiceName:    "test",
		DatabaseName:   "testdb",
		DatabaseSystem: DBSystemClickHouse,
		RecordSQL:      true,
		MaxSQLLength:   50,
	})

	ctx := context.Background()
	longSQL := "SELECT id, name, email, created_at, updated_at, status FROM users WHERE id = 1"

	ctx, finish := tracer.TraceQuery(ctx, "test", longSQL)
	finish(1, nil)

	// The SQL should have been truncated in the span
	// We can't easily verify this without a custom exporter, but the test verifies no panic
}

func TestSamplingStrategies(t *testing.T) {
	strategies := []struct {
		name     string
		strategy string
		ratio    float64
	}{
		{"always_on", "always_on", 0},
		{"always_off", "always_off", 0},
		{"trace_id_ratio", "trace_id_ratio", 0.5},
		{"parent_based", "parent_based", 1.0},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			cfg := &SamplingConfig{
				Strategy: s.strategy,
				Ratio:    s.ratio,
			}
			sampler := createSampler(cfg)
			if sampler == nil {
				t.Error("expected non-nil sampler")
			}
		})
	}
}

func TestGlobalFunctions(t *testing.T) {
	ctx := context.Background()

	// Test without initialization
	tracer := GetTracer()
	if tracer == nil {
		t.Error("expected non-nil tracer from GetTracer")
	}

	// Test span creation
	ctx, span := StartSpanFromContext(ctx, "test")
	if span == nil {
		t.Error("expected non-nil span")
	}
	span.End()

	// Test span from context
	span2 := SpanFromContext(ctx)
	if span2 == nil {
		t.Error("expected non-nil span from context")
	}
}

func TestTraceHTTPClient(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := NewTraceHTTPClient(nil, "test-client")

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestPrometheusExporter(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Tracing: TracingConfig{
			Enabled: false,
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Exporter: "prometheus",
			Prometheus: PrometheusExporterConfig{
				Path:      "/metrics",
				Port:      0,
				Namespace: "test",
			},
		},
	}

	mp, err := NewMetricsProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mp.Shutdown(ctx)

	// Test prometheus handler
	handler := mp.PrometheusHandler()
	if handler == nil {
		t.Error("expected non-nil prometheus handler")
	}

	// Make a request to the handler
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// Benchmark tests

func BenchmarkHTTPMiddleware(b *testing.B) {
	cfg := DefaultHTTPMiddlewareConfig()
	middleware := HTTPMiddleware(cfg)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware(handler)
	req := httptest.NewRequest("GET", "/api/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
	}
}

func BenchmarkDBTracer(b *testing.B) {
	tracer := NewClickHouseQueryTracer("testdb", true)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, finish := tracer.TraceSelect(ctx, "users", "SELECT * FROM users")
		finish(10, nil)
	}
}

func BenchmarkMetricsRecordQuery(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Tracing: TracingConfig{
			Enabled: false,
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Exporter: "none",
		},
	}

	mp, _ := NewMetricsProvider(ctx, cfg)
	defer mp.Shutdown(ctx)

	duration := 100 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mp.RecordQuery(ctx, "users", "SELECT", duration, 10, nil)
	}
}
