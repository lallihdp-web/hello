package telemetry

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DBSystemClickHouse is the ClickHouse database system identifier
	DBSystemClickHouse = "clickhouse"
)

// DBTracer provides database tracing utilities
type DBTracer struct {
	tracer       trace.Tracer
	dbName       string
	dbSystem     string
	recordSQL    bool
	maxSQLLength int
}

// DBTracerConfig holds configuration for database tracing
type DBTracerConfig struct {
	// Service name for the tracer
	ServiceName string

	// Database name
	DatabaseName string

	// Database system (e.g., "clickhouse", "postgresql")
	DatabaseSystem string

	// Whether to record SQL statements in spans
	RecordSQL bool

	// Maximum SQL length to record (0 = unlimited)
	MaxSQLLength int
}

// NewDBTracer creates a new database tracer
func NewDBTracer(cfg *DBTracerConfig) *DBTracer {
	if cfg == nil {
		cfg = &DBTracerConfig{
			ServiceName:    "ndc-clickhouse",
			DatabaseName:   "default",
			DatabaseSystem: DBSystemClickHouse,
			RecordSQL:      true,
			MaxSQLLength:   1000,
		}
	}

	return &DBTracer{
		tracer:       otel.Tracer(cfg.ServiceName),
		dbName:       cfg.DatabaseName,
		dbSystem:     cfg.DatabaseSystem,
		recordSQL:    cfg.RecordSQL,
		maxSQLLength: cfg.MaxSQLLength,
	}
}

// TraceQuery traces a database query execution
func (t *DBTracer) TraceQuery(ctx context.Context, operationName, sql string) (context.Context, func(rowCount int64, err error)) {
	ctx, span := t.tracer.Start(ctx, operationName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String(t.dbSystem),
			semconv.DBName(t.dbName),
			attribute.String("db.operation", operationName),
		),
	)

	if t.recordSQL {
		sqlToRecord := sql
		if t.maxSQLLength > 0 && len(sql) > t.maxSQLLength {
			sqlToRecord = sql[:t.maxSQLLength] + "..."
		}
		span.SetAttributes(semconv.DBStatement(sqlToRecord))
	}

	start := time.Now()

	return ctx, func(rowCount int64, err error) {
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Float64("db.duration_ms", float64(duration.Milliseconds())),
		)

		if rowCount >= 0 {
			span.SetAttributes(attribute.Int64("db.rows_affected", rowCount))
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		span.End()

		// Record metrics
		if globalMetricsProvider != nil {
			globalMetricsProvider.RecordQuery(ctx, t.dbName, operationName, duration, rowCount, err)
		}
	}
}

// TraceQueryWithCollection traces a query with collection info
func (t *DBTracer) TraceQueryWithCollection(ctx context.Context, collection, operationName, sql string) (context.Context, func(rowCount int64, err error)) {
	ctx, span := t.tracer.Start(ctx, operationName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String(t.dbSystem),
			semconv.DBName(t.dbName),
			attribute.String("db.operation", operationName),
			attribute.String("db.collection", collection),
		),
	)

	if t.recordSQL {
		sqlToRecord := sql
		if t.maxSQLLength > 0 && len(sql) > t.maxSQLLength {
			sqlToRecord = sql[:t.maxSQLLength] + "..."
		}
		span.SetAttributes(semconv.DBStatement(sqlToRecord))
	}

	start := time.Now()

	return ctx, func(rowCount int64, err error) {
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Float64("db.duration_ms", float64(duration.Milliseconds())),
		)

		if rowCount >= 0 {
			span.SetAttributes(attribute.Int64("db.rows_affected", rowCount))
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		span.End()

		// Record metrics
		if globalMetricsProvider != nil {
			globalMetricsProvider.RecordQuery(ctx, collection, operationName, duration, rowCount, err)
		}
	}
}

// TraceBatch traces a batch operation
func (t *DBTracer) TraceBatch(ctx context.Context, operationName string, batchSize int) (context.Context, func(err error)) {
	ctx, span := t.tracer.Start(ctx, operationName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String(t.dbSystem),
			semconv.DBName(t.dbName),
			attribute.String("db.operation", operationName),
			attribute.Int("db.batch_size", batchSize),
		),
	)

	start := time.Now()

	return ctx, func(err error) {
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Float64("db.duration_ms", float64(duration.Milliseconds())),
		)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		span.End()

		// Record metrics
		if globalMetricsProvider != nil {
			globalMetricsProvider.RecordQuery(ctx, t.dbName, operationName, duration, int64(batchSize), err)
		}
	}
}

// TraceTransaction traces a database transaction
func (t *DBTracer) TraceTransaction(ctx context.Context) (context.Context, func(err error)) {
	ctx, span := t.tracer.Start(ctx, "db.transaction",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String(t.dbSystem),
			semconv.DBName(t.dbName),
			attribute.String("db.operation", "transaction"),
		),
	)

	start := time.Now()

	return ctx, func(err error) {
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Float64("db.duration_ms", float64(duration.Milliseconds())),
		)

		if err != nil {
			span.SetAttributes(attribute.String("db.transaction.status", "rollback"))
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetAttributes(attribute.String("db.transaction.status", "commit"))
			span.SetStatus(codes.Ok, "")
		}

		span.End()
	}
}

// WrapDB wraps a sql.DB to add tracing
type TracedDB struct {
	DB     *sql.DB
	tracer *DBTracer
}

// NewTracedDB creates a traced database wrapper
func NewTracedDB(db *sql.DB, tracer *DBTracer) *TracedDB {
	return &TracedDB{
		DB:     db,
		tracer: tracer,
	}
}

// QueryContext executes a traced query
func (tdb *TracedDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	ctx, finish := tdb.tracer.TraceQuery(ctx, "db.query", query)

	rows, err := tdb.DB.QueryContext(ctx, query, args...)

	// We can't know row count until rows are consumed
	finish(-1, err)

	return rows, err
}

// ExecContext executes a traced exec
func (tdb *TracedDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	ctx, finish := tdb.tracer.TraceQuery(ctx, "db.exec", query)

	result, err := tdb.DB.ExecContext(ctx, query, args...)

	var rowCount int64 = -1
	if result != nil {
		rowCount, _ = result.RowsAffected()
	}

	finish(rowCount, err)

	return result, err
}

// ClickHouseQueryTracer provides ClickHouse-specific tracing
type ClickHouseQueryTracer struct {
	*DBTracer
}

// NewClickHouseQueryTracer creates a ClickHouse query tracer
func NewClickHouseQueryTracer(dbName string, recordSQL bool) *ClickHouseQueryTracer {
	return &ClickHouseQueryTracer{
		DBTracer: NewDBTracer(&DBTracerConfig{
			ServiceName:    "ndc-clickhouse",
			DatabaseName:   dbName,
			DatabaseSystem: DBSystemClickHouse,
			RecordSQL:      recordSQL,
			MaxSQLLength:   2000,
		}),
	}
}

// TraceSelect traces a SELECT query
func (t *ClickHouseQueryTracer) TraceSelect(ctx context.Context, collection, sql string) (context.Context, func(rowCount int64, err error)) {
	return t.TraceQueryWithCollection(ctx, collection, "SELECT", sql)
}

// TraceInsert traces an INSERT query
func (t *ClickHouseQueryTracer) TraceInsert(ctx context.Context, collection, sql string) (context.Context, func(rowCount int64, err error)) {
	return t.TraceQueryWithCollection(ctx, collection, "INSERT", sql)
}

// TraceAggregate traces an aggregate query
func (t *ClickHouseQueryTracer) TraceAggregate(ctx context.Context, collection, sql string) (context.Context, func(rowCount int64, err error)) {
	return t.TraceQueryWithCollection(ctx, collection, "AGGREGATE", sql)
}

// Global database tracer
var globalDBTracer *ClickHouseQueryTracer

// SetGlobalDBTracer sets the global database tracer
func SetGlobalDBTracer(tracer *ClickHouseQueryTracer) {
	globalDBTracer = tracer
}

// GetDBTracer returns the global database tracer
func GetDBTracer() *ClickHouseQueryTracer {
	return globalDBTracer
}

// TraceSelectQuery is a convenience function for tracing SELECT queries
func TraceSelectQuery(ctx context.Context, collection, sql string) (context.Context, func(rowCount int64, err error)) {
	if globalDBTracer != nil {
		return globalDBTracer.TraceSelect(ctx, collection, sql)
	}
	return ctx, func(int64, error) {}
}

// TraceInsertQuery is a convenience function for tracing INSERT queries
func TraceInsertQuery(ctx context.Context, collection, sql string) (context.Context, func(rowCount int64, err error)) {
	if globalDBTracer != nil {
		return globalDBTracer.TraceInsert(ctx, collection, sql)
	}
	return ctx, func(int64, error) {}
}
