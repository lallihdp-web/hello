/**
 * DuckDB-WASM Module for Client-Side Analytics
 *
 * This module provides:
 * - DuckDBClient: Core client for interacting with DuckDB-WASM
 * - React hooks for declarative data loading and querying
 * - Hybrid analytics (compare local vs remote performance)
 *
 * Usage:
 * ```tsx
 * import { useDuckDB, useDuckDBQuery, useDuckDBLoader } from '@/lib/duckdb';
 *
 * function MyComponent() {
 *   const { initialized } = useDuckDB();
 *   const { loadData } = useDuckDBLoader();
 *   const { data } = useDuckDBQuery('SELECT * FROM my_table');
 *
 *   // ...
 * }
 * ```
 */

// Client
export {
  default as DuckDBClient,
  getDuckDBClient,
  resetDuckDBClient,
} from './client';

export type {
  DuckDBConfig,
  QueryResult,
  CachedTable,
  DuckDBStats,
} from './client';

// Hooks
export {
  useDuckDB,
  useDuckDBQuery,
  useDuckDBLoader,
  useDuckDBStats,
  useDuckDBAggregation,
  useDuckDBTables,
  useDuckDBSync,
  useDuckDBComparison,
} from './hooks';
