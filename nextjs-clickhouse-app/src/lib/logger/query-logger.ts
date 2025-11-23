/**
 * GraphQL Query Logger
 *
 * Provides configurable logging for GraphQL queries with slow query detection,
 * statistics collection, and optimization hints.
 */

import {
  QueryLoggerConfig,
  QueryLogEntry,
  QueryStats,
  OperationStats,
  LogLevel,
  defaultConfig,
  createConfig,
} from './config';

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
  none: 4,
};

/**
 * Query Logger class for tracking and analyzing GraphQL queries
 */
export class QueryLogger {
  private config: QueryLoggerConfig;
  private durations: number[] = [];
  private slowQueryHistory: QueryLogEntry[] = [];
  private operationStats: Map<string, OperationStats> = new Map();
  private totalQueries = 0;
  private successfulQueries = 0;
  private failedQueries = 0;
  private slowQueries = 0;
  private verySlowQueries = 0;
  private criticalQueries = 0;
  private windowStart: Date = new Date();

  constructor(config: Partial<QueryLoggerConfig> = {}) {
    this.config = createConfig(config);
  }

  /**
   * Update logger configuration
   */
  updateConfig(config: Partial<QueryLoggerConfig>): void {
    this.config = createConfig({ ...this.config, ...config });
  }

  /**
   * Get current configuration
   */
  getConfig(): QueryLoggerConfig {
    return { ...this.config };
  }

  /**
   * Check if logging is enabled for a given level
   */
  private shouldLog(level: LogLevel): boolean {
    if (!this.config.enabled) return false;
    return LOG_LEVELS[level] >= LOG_LEVELS[this.config.logLevel];
  }

  /**
   * Check if this query should be sampled
   */
  private shouldSample(): boolean {
    if (this.config.sampleRate >= 1) return true;
    if (this.config.sampleRate <= 0) return false;
    return Math.random() < this.config.sampleRate;
  }

  /**
   * Extract operation name from GraphQL query
   */
  private extractOperationName(query: string): string | null {
    const match = query.match(/(?:query|mutation|subscription)\s+(\w+)/);
    return match ? match[1] : null;
  }

  /**
   * Detect operation type from query
   */
  private detectOperationType(query: string): QueryLogEntry['operationType'] {
    const trimmed = query.trim().toLowerCase();
    if (trimmed.startsWith('mutation')) return 'mutation';
    if (trimmed.startsWith('subscription')) return 'subscription';
    if (trimmed.startsWith('query') || trimmed.startsWith('{')) return 'query';
    return 'unknown';
  }

  /**
   * Classify slow query level
   */
  private classifySlowLevel(durationMs: number): QueryLogEntry['slowLevel'] {
    const { slow, verySlow, critical } = this.config.slowQueryThresholds;
    if (durationMs >= critical) return 'critical';
    if (durationMs >= verySlow) return 'very_slow';
    if (durationMs >= slow) return 'slow';
    return 'normal';
  }

  /**
   * Generate optimization hints based on query analysis
   */
  private generateOptimizationHints(
    query: string,
    durationMs: number,
    slowLevel: QueryLogEntry['slowLevel'],
    responseSizeBytes?: number
  ): string[] {
    const hints: string[] = [];

    if (slowLevel === 'normal') return hints;

    // Analyze query structure
    const hasNoLimit = !query.toLowerCase().includes('limit');
    const hasSelectAll = query.includes('*') || query.match(/{\s*\w+\s*{/);
    const hasDeepNesting = (query.match(/{/g) || []).length > 4;
    const hasNoWhere = !query.toLowerCase().includes('where') &&
                       !query.toLowerCase().includes('filter');
    const hasOrderBy = query.toLowerCase().includes('order');
    const hasAggregation = /\b(count|sum|avg|min|max|group)\b/i.test(query);

    // Generate hints based on patterns
    if (hasNoLimit) {
      hints.push('PAGINATION: Add LIMIT clause to prevent fetching excessive rows');
    }

    if (hasSelectAll) {
      hints.push('SELECT FIELDS: Request only needed fields instead of selecting all');
    }

    if (hasDeepNesting) {
      hints.push('NESTING: Deep nesting detected - consider splitting into multiple queries');
    }

    if (hasNoWhere && durationMs > 1000) {
      hints.push('FILTERING: Add WHERE/filter conditions to reduce data scanned');
    }

    if (hasOrderBy && hasNoLimit) {
      hints.push('SORTING: ORDER BY without LIMIT causes full result sorting');
    }

    if (hasAggregation && durationMs > 2000) {
      hints.push('AGGREGATION: Consider pre-computed aggregations or materialized views');
    }

    // Response size hints
    if (responseSizeBytes && responseSizeBytes > 1024 * 1024) {
      hints.push(`RESPONSE SIZE: Large response (${(responseSizeBytes / 1024 / 1024).toFixed(2)}MB) - consider pagination`);
    }

    // Duration-specific hints
    if (slowLevel === 'critical') {
      hints.push('CRITICAL: Query exceeds critical threshold - immediate optimization required');
      hints.push('INDEX: Verify indexes exist for filtered/sorted columns in ClickHouse');
      hints.push('CACHE: Consider caching this query result with DuckDB query cache');
    } else if (slowLevel === 'very_slow') {
      hints.push('PERFORMANCE: Query is significantly slow - review execution plan');
      hints.push('PARTITIONING: Ensure query leverages ClickHouse partition pruning');
    }

    // ClickHouse-specific hints
    if (durationMs > this.config.slowQueryThresholds.slow) {
      hints.push('CLICKHOUSE: Check if query uses appropriate ClickHouse table engine (MergeTree family)');
      hints.push('COMPRESSION: Verify column compression settings for frequently queried columns');
    }

    return hints;
  }

  /**
   * Calculate response size from data
   */
  private calculateResponseSize(data: unknown): number {
    try {
      return new Blob([JSON.stringify(data)]).size;
    } catch {
      return 0;
    }
  }

  /**
   * Truncate query for logging
   */
  private truncateQuery(query: string): string {
    if (query.length <= this.config.maxQueryLength) return query;
    return query.substring(0, this.config.maxQueryLength) + '... [truncated]';
  }

  /**
   * Generate unique ID
   */
  private generateId(): string {
    return `q_${Date.now()}_${Math.random().toString(36).substring(2, 9)}`;
  }

  /**
   * Format log entry for console output
   */
  private formatLogEntry(entry: QueryLogEntry): string {
    const statusIcon = entry.success ? '✓' : '✗';
    const slowIcon = {
      normal: '',
      slow: '🐢',
      very_slow: '🐌',
      critical: '🔥',
    }[entry.slowLevel];

    let output = `[GraphQL ${entry.operationType.toUpperCase()}] ${statusIcon} ${slowIcon}`;
    output += `\n  Operation: ${entry.operationName || 'anonymous'}`;
    output += `\n  Duration: ${entry.durationMs.toFixed(2)}ms`;

    if (entry.responseSizeBytes !== undefined) {
      const sizeKB = entry.responseSizeBytes / 1024;
      output += `\n  Response Size: ${sizeKB.toFixed(2)}KB`;
    }

    if (!entry.success && entry.error) {
      output += `\n  Error: ${entry.error}`;
    }

    if (entry.optimizationHints && entry.optimizationHints.length > 0) {
      output += '\n  Optimization Hints:';
      entry.optimizationHints.forEach((hint) => {
        output += `\n    • ${hint}`;
      });
    }

    if (this.config.includeVariables && entry.variables) {
      output += `\n  Variables: ${JSON.stringify(entry.variables)}`;
    }

    return output;
  }

  /**
   * Output log entry
   */
  private outputLog(entry: QueryLogEntry, level: LogLevel): void {
    if (!this.shouldLog(level)) return;

    if (this.config.customLogger) {
      this.config.customLogger(entry);
      return;
    }

    const formatted = this.formatLogEntry(entry);

    switch (level) {
      case 'debug':
        console.debug(formatted);
        break;
      case 'info':
        console.info(formatted);
        break;
      case 'warn':
        console.warn(formatted);
        break;
      case 'error':
        console.error(formatted);
        break;
    }
  }

  /**
   * Update operation statistics
   */
  private updateOperationStats(entry: QueryLogEntry): void {
    const opName = entry.operationName || 'anonymous';
    const existing = this.operationStats.get(opName);

    if (existing) {
      const newCount = existing.count + 1;
      existing.count = newCount;
      existing.avgDurationMs =
        (existing.avgDurationMs * (newCount - 1) + entry.durationMs) / newCount;
      existing.maxDurationMs = Math.max(existing.maxDurationMs, entry.durationMs);
      existing.minDurationMs = Math.min(existing.minDurationMs, entry.durationMs);
      if (entry.slowLevel !== 'normal') existing.slowCount++;
      if (!entry.success) existing.errorCount++;
    } else {
      this.operationStats.set(opName, {
        count: 1,
        avgDurationMs: entry.durationMs,
        maxDurationMs: entry.durationMs,
        minDurationMs: entry.durationMs,
        slowCount: entry.slowLevel !== 'normal' ? 1 : 0,
        errorCount: entry.success ? 0 : 1,
      });
    }
  }

  /**
   * Calculate percentile from sorted array
   */
  private percentile(arr: number[], p: number): number {
    if (arr.length === 0) return 0;
    const sorted = [...arr].sort((a, b) => a - b);
    const index = Math.ceil((p / 100) * sorted.length) - 1;
    return sorted[Math.max(0, index)];
  }

  /**
   * Log a query execution
   */
  log(params: {
    query: string;
    variables?: Record<string, unknown>;
    durationMs: number;
    success: boolean;
    error?: string;
    responseData?: unknown;
    source?: string;
    metadata?: Record<string, unknown>;
  }): QueryLogEntry | null {
    if (!this.config.enabled) return null;
    if (!this.shouldSample()) return null;

    const {
      query,
      variables,
      durationMs,
      success,
      error,
      responseData,
      source = 'unknown',
      metadata,
    } = params;

    const slowLevel = this.classifySlowLevel(durationMs);
    const responseSizeBytes = this.config.includeResponseSize && responseData
      ? this.calculateResponseSize(responseData)
      : undefined;

    const entry: QueryLogEntry = {
      id: this.generateId(),
      timestamp: new Date(),
      operationName: this.extractOperationName(query),
      operationType: this.detectOperationType(query),
      query: this.truncateQuery(query),
      durationMs,
      success,
      slowLevel,
      source,
      ...(this.config.includeVariables && variables && { variables }),
      ...(responseSizeBytes !== undefined && { responseSizeBytes }),
      ...(error && { error }),
      ...(metadata && { metadata }),
    };

    // Generate optimization hints for slow queries
    if (this.config.includeOptimizationHints && slowLevel !== 'normal') {
      entry.optimizationHints = this.generateOptimizationHints(
        query,
        durationMs,
        slowLevel,
        responseSizeBytes
      );
    }

    // Update statistics
    if (this.config.collectStats) {
      this.totalQueries++;
      this.durations.push(durationMs);

      if (success) {
        this.successfulQueries++;
      } else {
        this.failedQueries++;
      }

      switch (slowLevel) {
        case 'slow':
          this.slowQueries++;
          break;
        case 'very_slow':
          this.verySlowQueries++;
          break;
        case 'critical':
          this.criticalQueries++;
          break;
      }

      this.updateOperationStats(entry);
    }

    // Store slow queries in history
    if (slowLevel !== 'normal') {
      this.slowQueryHistory.push(entry);
      if (this.slowQueryHistory.length > this.config.slowQueryHistorySize) {
        this.slowQueryHistory.shift();
      }

      // Call slow query callback if configured
      if (this.config.onSlowQuery) {
        this.config.onSlowQuery(entry);
      }
    }

    // Determine log level and output
    let logLevel: LogLevel = 'debug';
    if (!success) {
      logLevel = 'error';
    } else if (slowLevel === 'critical') {
      logLevel = 'error';
    } else if (slowLevel === 'very_slow') {
      logLevel = 'warn';
    } else if (slowLevel === 'slow') {
      logLevel = 'warn';
    } else if (this.config.logAllQueries) {
      logLevel = 'info';
    }

    // Only log if configured to log all queries or if it's slow
    if (this.config.logAllQueries || slowLevel !== 'normal' || !success) {
      this.outputLog(entry, logLevel);
    }

    return entry;
  }

  /**
   * Create a timer for measuring query duration
   */
  startTimer(): () => number {
    const start = performance.now();
    return () => performance.now() - start;
  }

  /**
   * Get query statistics
   */
  getStats(): QueryStats {
    return {
      totalQueries: this.totalQueries,
      successfulQueries: this.successfulQueries,
      failedQueries: this.failedQueries,
      slowQueries: this.slowQueries,
      verySlowQueries: this.verySlowQueries,
      criticalQueries: this.criticalQueries,
      avgDurationMs: this.durations.length > 0
        ? this.durations.reduce((a, b) => a + b, 0) / this.durations.length
        : 0,
      maxDurationMs: this.durations.length > 0 ? Math.max(...this.durations) : 0,
      minDurationMs: this.durations.length > 0 ? Math.min(...this.durations) : 0,
      p50DurationMs: this.percentile(this.durations, 50),
      p90DurationMs: this.percentile(this.durations, 90),
      p95DurationMs: this.percentile(this.durations, 95),
      p99DurationMs: this.percentile(this.durations, 99),
      queriesByOperation: new Map(this.operationStats),
      windowStart: this.windowStart,
      windowEnd: new Date(),
    };
  }

  /**
   * Get slow query history
   */
  getSlowQueryHistory(): QueryLogEntry[] {
    return [...this.slowQueryHistory];
  }

  /**
   * Get summary report of slow queries
   */
  getSlowQueryReport(): string {
    const stats = this.getStats();
    const history = this.getSlowQueryHistory();

    let report = '=== GraphQL Query Performance Report ===\n\n';

    report += '📊 Overall Statistics:\n';
    report += `  Total Queries: ${stats.totalQueries}\n`;
    report += `  Success Rate: ${((stats.successfulQueries / stats.totalQueries) * 100).toFixed(1)}%\n`;
    report += `  Avg Duration: ${stats.avgDurationMs.toFixed(2)}ms\n`;
    report += `  P50: ${stats.p50DurationMs.toFixed(2)}ms\n`;
    report += `  P90: ${stats.p90DurationMs.toFixed(2)}ms\n`;
    report += `  P95: ${stats.p95DurationMs.toFixed(2)}ms\n`;
    report += `  P99: ${stats.p99DurationMs.toFixed(2)}ms\n\n`;

    report += '🐢 Slow Query Summary:\n';
    report += `  Slow (>${this.config.slowQueryThresholds.slow}ms): ${stats.slowQueries}\n`;
    report += `  Very Slow (>${this.config.slowQueryThresholds.verySlow}ms): ${stats.verySlowQueries}\n`;
    report += `  Critical (>${this.config.slowQueryThresholds.critical}ms): ${stats.criticalQueries}\n\n`;

    if (stats.queriesByOperation.size > 0) {
      report += '📋 By Operation:\n';
      const sortedOps = [...stats.queriesByOperation.entries()]
        .sort((a, b) => b[1].avgDurationMs - a[1].avgDurationMs);

      for (const [name, opStats] of sortedOps.slice(0, 10)) {
        report += `  ${name}:\n`;
        report += `    Count: ${opStats.count}, Avg: ${opStats.avgDurationMs.toFixed(2)}ms, `;
        report += `Max: ${opStats.maxDurationMs.toFixed(2)}ms, Slow: ${opStats.slowCount}\n`;
      }
      report += '\n';
    }

    if (history.length > 0) {
      report += '🔥 Recent Slow Queries:\n';
      const recentSlow = history.slice(-5);
      for (const entry of recentSlow) {
        report += `  [${entry.timestamp.toISOString()}] ${entry.operationName || 'anonymous'}\n`;
        report += `    Duration: ${entry.durationMs.toFixed(2)}ms (${entry.slowLevel})\n`;
        if (entry.optimizationHints && entry.optimizationHints.length > 0) {
          report += `    Hints: ${entry.optimizationHints[0]}\n`;
        }
      }
    }

    return report;
  }

  /**
   * Reset all statistics
   */
  resetStats(): void {
    this.durations = [];
    this.slowQueryHistory = [];
    this.operationStats.clear();
    this.totalQueries = 0;
    this.successfulQueries = 0;
    this.failedQueries = 0;
    this.slowQueries = 0;
    this.verySlowQueries = 0;
    this.criticalQueries = 0;
    this.windowStart = new Date();
  }
}

// Global singleton instance
let globalLogger: QueryLogger | null = null;

/**
 * Get or create the global query logger instance
 */
export function getQueryLogger(config?: Partial<QueryLoggerConfig>): QueryLogger {
  if (!globalLogger) {
    globalLogger = new QueryLogger(config);
  } else if (config) {
    globalLogger.updateConfig(config);
  }
  return globalLogger;
}

/**
 * Create a new query logger instance (not singleton)
 */
export function createQueryLogger(config?: Partial<QueryLoggerConfig>): QueryLogger {
  return new QueryLogger(config);
}

export default QueryLogger;
