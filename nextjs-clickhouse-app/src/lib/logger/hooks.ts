/**
 * React Hooks for GraphQL Query Logger
 *
 * Provides hooks for accessing query statistics, slow query history,
 * and real-time query performance monitoring.
 */

'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { getQueryLogger, QueryLogger } from './query-logger';
import type { QueryStats, QueryLogEntry, QueryLoggerConfig } from './config';

/**
 * Hook to access the global query logger instance
 */
export function useQueryLogger(config?: Partial<QueryLoggerConfig>): QueryLogger {
  const logger = useMemo(() => getQueryLogger(config), []);

  useEffect(() => {
    if (config) {
      logger.updateConfig(config);
    }
  }, [logger, config]);

  return logger;
}

/**
 * Hook to get query statistics with auto-refresh
 */
export function useQueryStats(refreshIntervalMs = 5000): {
  stats: QueryStats | null;
  refresh: () => void;
  reset: () => void;
} {
  const logger = useQueryLogger();
  const [stats, setStats] = useState<QueryStats | null>(null);

  const refresh = useCallback(() => {
    setStats(logger.getStats());
  }, [logger]);

  const reset = useCallback(() => {
    logger.resetStats();
    setStats(logger.getStats());
  }, [logger]);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, refreshIntervalMs);
    return () => clearInterval(interval);
  }, [refresh, refreshIntervalMs]);

  return { stats, refresh, reset };
}

/**
 * Hook to get slow query history with auto-refresh
 */
export function useSlowQueryHistory(refreshIntervalMs = 5000): {
  history: QueryLogEntry[];
  refresh: () => void;
} {
  const logger = useQueryLogger();
  const [history, setHistory] = useState<QueryLogEntry[]>([]);

  const refresh = useCallback(() => {
    setHistory(logger.getSlowQueryHistory());
  }, [logger]);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, refreshIntervalMs);
    return () => clearInterval(interval);
  }, [refresh, refreshIntervalMs]);

  return { history, refresh };
}

/**
 * Hook to get a formatted slow query report
 */
export function useSlowQueryReport(): {
  report: string;
  refresh: () => void;
} {
  const logger = useQueryLogger();
  const [report, setReport] = useState<string>('');

  const refresh = useCallback(() => {
    setReport(logger.getSlowQueryReport());
  }, [logger]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { report, refresh };
}

/**
 * Hook for real-time slow query notifications
 */
export function useSlowQueryNotifications(
  onSlowQuery: (entry: QueryLogEntry) => void
): void {
  const logger = useQueryLogger();

  useEffect(() => {
    logger.updateConfig({ onSlowQuery });

    return () => {
      logger.updateConfig({ onSlowQuery: undefined });
    };
  }, [logger, onSlowQuery]);
}

/**
 * Hook for query performance summary
 */
export function useQueryPerformanceSummary(): {
  totalQueries: number;
  slowQueryCount: number;
  avgDuration: number;
  p95Duration: number;
  errorRate: number;
} {
  const { stats } = useQueryStats(3000);

  return useMemo(() => {
    if (!stats) {
      return {
        totalQueries: 0,
        slowQueryCount: 0,
        avgDuration: 0,
        p95Duration: 0,
        errorRate: 0,
      };
    }

    return {
      totalQueries: stats.totalQueries,
      slowQueryCount: stats.slowQueries + stats.verySlowQueries + stats.criticalQueries,
      avgDuration: stats.avgDurationMs,
      p95Duration: stats.p95DurationMs,
      errorRate: stats.totalQueries > 0
        ? (stats.failedQueries / stats.totalQueries) * 100
        : 0,
    };
  }, [stats]);
}

/**
 * Hook for operation-specific statistics
 */
export function useOperationStats(operationName: string): OperationStatistics | null {
  const { stats } = useQueryStats();

  return useMemo(() => {
    if (!stats) return null;
    const opStats = stats.queriesByOperation.get(operationName);
    if (!opStats) return null;

    return {
      name: operationName,
      ...opStats,
      slowRate: opStats.count > 0 ? (opStats.slowCount / opStats.count) * 100 : 0,
      errorRate: opStats.count > 0 ? (opStats.errorCount / opStats.count) * 100 : 0,
    };
  }, [stats, operationName]);
}

interface OperationStatistics {
  name: string;
  count: number;
  avgDurationMs: number;
  maxDurationMs: number;
  minDurationMs: number;
  slowCount: number;
  errorCount: number;
  slowRate: number;
  errorRate: number;
}

/**
 * Hook for top slow operations
 */
export function useTopSlowOperations(limit = 10): OperationStatistics[] {
  const { stats } = useQueryStats();

  return useMemo(() => {
    if (!stats) return [];

    return Array.from(stats.queriesByOperation.entries())
      .map(([name, opStats]) => ({
        name,
        ...opStats,
        slowRate: opStats.count > 0 ? (opStats.slowCount / opStats.count) * 100 : 0,
        errorRate: opStats.count > 0 ? (opStats.errorCount / opStats.count) * 100 : 0,
      }))
      .sort((a, b) => b.avgDurationMs - a.avgDurationMs)
      .slice(0, limit);
  }, [stats, limit]);
}

/**
 * Hook for logger configuration management
 */
export function useQueryLoggerConfig(): {
  config: QueryLoggerConfig;
  updateConfig: (updates: Partial<QueryLoggerConfig>) => void;
  enable: () => void;
  disable: () => void;
  setLogLevel: (level: QueryLoggerConfig['logLevel']) => void;
  setSlowThreshold: (thresholdMs: number) => void;
} {
  const logger = useQueryLogger();
  const [config, setConfig] = useState<QueryLoggerConfig>(logger.getConfig());

  const updateConfig = useCallback((updates: Partial<QueryLoggerConfig>) => {
    logger.updateConfig(updates);
    setConfig(logger.getConfig());
  }, [logger]);

  const enable = useCallback(() => updateConfig({ enabled: true }), [updateConfig]);
  const disable = useCallback(() => updateConfig({ enabled: false }), [updateConfig]);

  const setLogLevel = useCallback(
    (level: QueryLoggerConfig['logLevel']) => updateConfig({ logLevel: level }),
    [updateConfig]
  );

  const setSlowThreshold = useCallback(
    (thresholdMs: number) =>
      updateConfig({
        slowQueryThresholds: { ...config.slowQueryThresholds, slow: thresholdMs },
      }),
    [updateConfig, config.slowQueryThresholds]
  );

  return {
    config,
    updateConfig,
    enable,
    disable,
    setLogLevel,
    setSlowThreshold,
  };
}
