/**
 * GraphQL Query Logger Module
 *
 * Provides configurable logging for GraphQL queries with:
 * - Slow query detection and alerts
 * - Performance statistics collection
 * - Optimization hints for database tuning
 * - Configurable thresholds and log levels
 */

export {
  QueryLogger,
  getQueryLogger,
  createQueryLogger,
} from './query-logger';

export {
  defaultConfig,
  createConfig,
} from './config';

export {
  useQueryLogger,
  useQueryStats,
  useSlowQueryHistory,
  useSlowQueryReport,
  useSlowQueryNotifications,
  useQueryPerformanceSummary,
  useOperationStats,
  useTopSlowOperations,
  useQueryLoggerConfig,
} from './hooks';

export type {
  LogLevel,
  SlowQueryThresholds,
  QueryLoggerConfig,
  QueryLogEntry,
  QueryStats,
  OperationStats,
} from './config';
