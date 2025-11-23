/**
 * GraphQL Query Logger Configuration
 *
 * Configurable logging for GraphQL queries with slow query detection
 * and optimization hints.
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error' | 'none';

export interface SlowQueryThresholds {
  /** Queries taking longer than this (ms) are considered slow */
  slow: number;
  /** Queries taking longer than this (ms) are considered very slow */
  verySlow: number;
  /** Queries taking longer than this (ms) are considered critical */
  critical: number;
}

export interface QueryLoggerConfig {
  /** Enable or disable query logging */
  enabled: boolean;

  /** Minimum log level to output */
  logLevel: LogLevel;

  /** Log all queries (true) or only slow ones (false) */
  logAllQueries: boolean;

  /** Thresholds for slow query detection (in milliseconds) */
  slowQueryThresholds: SlowQueryThresholds;

  /** Include query variables in logs (may contain sensitive data) */
  includeVariables: boolean;

  /** Include response data size in logs */
  includeResponseSize: boolean;

  /** Include optimization hints for slow queries */
  includeOptimizationHints: boolean;

  /** Maximum query length to log (truncates longer queries) */
  maxQueryLength: number;

  /** Enable query statistics collection */
  collectStats: boolean;

  /** Number of slow queries to keep in history */
  slowQueryHistorySize: number;

  /** Custom log output function */
  customLogger?: (entry: QueryLogEntry) => void;

  /** Callback for slow queries (useful for alerts) */
  onSlowQuery?: (entry: QueryLogEntry) => void;

  /** Sample rate for logging (0-1, 1 = log all, 0.1 = log 10%) */
  sampleRate: number;
}

export interface QueryLogEntry {
  /** Unique query execution ID */
  id: string;

  /** Timestamp when query started */
  timestamp: Date;

  /** Operation name from query */
  operationName: string | null;

  /** Operation type: query, mutation, subscription */
  operationType: 'query' | 'mutation' | 'subscription' | 'unknown';

  /** The GraphQL query string */
  query: string;

  /** Query variables (if includeVariables is enabled) */
  variables?: Record<string, unknown>;

  /** Execution duration in milliseconds */
  durationMs: number;

  /** Response size in bytes (if includeResponseSize is enabled) */
  responseSizeBytes?: number;

  /** Whether the query succeeded */
  success: boolean;

  /** Error message if query failed */
  error?: string;

  /** Slow query classification */
  slowLevel: 'normal' | 'slow' | 'very_slow' | 'critical';

  /** Optimization hints (if includeOptimizationHints is enabled) */
  optimizationHints?: string[];

  /** Source of the query (apollo, fetch, etc.) */
  source: string;

  /** Additional metadata */
  metadata?: Record<string, unknown>;
}

export interface QueryStats {
  /** Total queries executed */
  totalQueries: number;

  /** Total successful queries */
  successfulQueries: number;

  /** Total failed queries */
  failedQueries: number;

  /** Total slow queries (above slow threshold) */
  slowQueries: number;

  /** Total very slow queries */
  verySlowQueries: number;

  /** Total critical queries */
  criticalQueries: number;

  /** Average query duration in ms */
  avgDurationMs: number;

  /** Maximum query duration in ms */
  maxDurationMs: number;

  /** Minimum query duration in ms */
  minDurationMs: number;

  /** 50th percentile (median) duration */
  p50DurationMs: number;

  /** 90th percentile duration */
  p90DurationMs: number;

  /** 95th percentile duration */
  p95DurationMs: number;

  /** 99th percentile duration */
  p99DurationMs: number;

  /** Queries per operation name */
  queriesByOperation: Map<string, OperationStats>;

  /** Time window start */
  windowStart: Date;

  /** Time window end */
  windowEnd: Date;
}

export interface OperationStats {
  count: number;
  avgDurationMs: number;
  maxDurationMs: number;
  minDurationMs: number;
  slowCount: number;
  errorCount: number;
}

/**
 * Default configuration values
 */
export const defaultConfig: QueryLoggerConfig = {
  enabled: process.env.NODE_ENV !== 'production' ||
           process.env.NEXT_PUBLIC_ENABLE_QUERY_LOGGING === 'true',
  logLevel: (process.env.NEXT_PUBLIC_QUERY_LOG_LEVEL as LogLevel) || 'info',
  logAllQueries: process.env.NEXT_PUBLIC_LOG_ALL_QUERIES === 'true',
  slowQueryThresholds: {
    slow: parseInt(process.env.NEXT_PUBLIC_SLOW_QUERY_THRESHOLD || '500', 10),
    verySlow: parseInt(process.env.NEXT_PUBLIC_VERY_SLOW_QUERY_THRESHOLD || '2000', 10),
    critical: parseInt(process.env.NEXT_PUBLIC_CRITICAL_QUERY_THRESHOLD || '5000', 10),
  },
  includeVariables: process.env.NEXT_PUBLIC_LOG_QUERY_VARIABLES === 'true',
  includeResponseSize: true,
  includeOptimizationHints: true,
  maxQueryLength: 1000,
  collectStats: true,
  slowQueryHistorySize: 100,
  sampleRate: parseFloat(process.env.NEXT_PUBLIC_QUERY_LOG_SAMPLE_RATE || '1'),
};

/**
 * Create a custom configuration by merging with defaults
 */
export function createConfig(overrides: Partial<QueryLoggerConfig>): QueryLoggerConfig {
  return {
    ...defaultConfig,
    ...overrides,
    slowQueryThresholds: {
      ...defaultConfig.slowQueryThresholds,
      ...overrides.slowQueryThresholds,
    },
  };
}
