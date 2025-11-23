/**
 * React Hooks for DuckDB-WASM
 *
 * Provides React hooks for client-side analytics with DuckDB-WASM.
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import { getDuckDBClient, QueryResult, DuckDBStats, CachedTable } from './client';

/**
 * Hook to initialize DuckDB
 */
export function useDuckDB() {
  const [initialized, setInitialized] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const initRef = useRef(false);

  useEffect(() => {
    // Prevent double initialization in strict mode
    if (initRef.current) return;
    initRef.current = true;

    const init = async () => {
      try {
        const client = getDuckDBClient();
        await client.initialize();
        setInitialized(true);
      } catch (err) {
        setError(err instanceof Error ? err : new Error(String(err)));
      } finally {
        setLoading(false);
      }
    };

    init();
  }, []);

  return { initialized, loading, error };
}

/**
 * Hook to execute DuckDB queries
 */
export function useDuckDBQuery<T = Record<string, unknown>>(
  sql: string | null,
  options: {
    skip?: boolean;
    refreshInterval?: number;
  } = {}
) {
  const [data, setData] = useState<QueryResult<T> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const { skip = false, refreshInterval } = options;

  const execute = useCallback(async () => {
    if (!sql || skip) return;

    setLoading(true);
    setError(null);

    try {
      const client = getDuckDBClient();
      const result = await client.query<T>(sql);
      setData(result);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [sql, skip]);

  useEffect(() => {
    execute();
  }, [execute]);

  // Optional refresh interval
  useEffect(() => {
    if (!refreshInterval || skip) return;

    const interval = setInterval(execute, refreshInterval);
    return () => clearInterval(interval);
  }, [refreshInterval, skip, execute]);

  const refetch = useCallback(() => {
    execute();
  }, [execute]);

  return { data, loading, error, refetch };
}

/**
 * Hook to load data into DuckDB
 */
export function useDuckDBLoader() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const loadData = useCallback(
    async (tableName: string, data: Record<string, unknown>[]) => {
      setLoading(true);
      setError(null);

      try {
        const client = getDuckDBClient();
        await client.loadData(tableName, data);
      } catch (err) {
        setError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      } finally {
        setLoading(false);
      }
    },
    []
  );

  const loadFromParquet = useCallback(async (tableName: string, url: string) => {
    setLoading(true);
    setError(null);

    try {
      const client = getDuckDBClient();
      await client.loadFromParquet(tableName, url);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const loadFromCSV = useCallback(
    async (tableName: string, csvContent: string) => {
      setLoading(true);
      setError(null);

      try {
        const client = getDuckDBClient();
        await client.loadFromCSV(tableName, csvContent);
      } catch (err) {
        setError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      } finally {
        setLoading(false);
      }
    },
    []
  );

  return { loadData, loadFromParquet, loadFromCSV, loading, error };
}

/**
 * Hook to get DuckDB statistics
 */
export function useDuckDBStats() {
  const [stats, setStats] = useState<DuckDBStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const client = getDuckDBClient();
      const newStats = await client.getStats();
      setStats(newStats);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { stats, loading, error, refresh };
}

/**
 * Hook for analytics aggregations
 */
export function useDuckDBAggregation<T = Record<string, unknown>>(
  tableName: string | null,
  options: {
    groupBy?: string[];
    aggregations: Array<{
      column: string;
      function: 'sum' | 'avg' | 'count' | 'min' | 'max';
      alias?: string;
    }>;
    filters?: Record<string, unknown>;
    orderBy?: string;
    limit?: number;
    skip?: boolean;
  }
) {
  const [data, setData] = useState<QueryResult<T> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const { skip = false, ...aggOptions } = options;

  const execute = useCallback(async () => {
    if (!tableName || skip) return;

    setLoading(true);
    setError(null);

    try {
      const client = getDuckDBClient();
      const result = await client.aggregate<T>(tableName, aggOptions);
      setData(result);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [tableName, skip, JSON.stringify(aggOptions)]);

  useEffect(() => {
    execute();
  }, [execute]);

  const refetch = useCallback(() => {
    execute();
  }, [execute]);

  return { data, loading, error, refetch };
}

/**
 * Hook to manage cached tables
 */
export function useDuckDBTables() {
  const [tables, setTables] = useState<CachedTable[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const client = getDuckDBClient();
      const newTables = await client.getTables();
      setTables(newTables);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const dropTable = useCallback(
    async (tableName: string) => {
      try {
        const client = getDuckDBClient();
        await client.dropTable(tableName);
        await refresh();
      } catch (err) {
        setError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    [refresh]
  );

  const clearAll = useCallback(async () => {
    try {
      const client = getDuckDBClient();
      await client.clearAll();
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, [refresh]);

  return { tables, loading, error, refresh, dropTable, clearAll };
}

/**
 * Hook to sync data from server to DuckDB
 */
export function useDuckDBSync(
  serverFetch: () => Promise<{ tableName: string; data: Record<string, unknown>[] }[]>,
  options: {
    autoSync?: boolean;
    syncInterval?: number;
  } = {}
) {
  const [syncing, setSyncing] = useState(false);
  const [lastSync, setLastSync] = useState<Date | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const { autoSync = false, syncInterval = 60000 } = options;

  const sync = useCallback(async () => {
    setSyncing(true);
    setError(null);

    try {
      const client = getDuckDBClient();
      const datasets = await serverFetch();

      for (const { tableName, data } of datasets) {
        await client.loadData(tableName, data);
      }

      setLastSync(new Date());
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    } finally {
      setSyncing(false);
    }
  }, [serverFetch]);

  // Auto-sync on mount if enabled
  useEffect(() => {
    if (autoSync) {
      sync();
    }
  }, [autoSync, sync]);

  // Periodic sync
  useEffect(() => {
    if (!autoSync || !syncInterval) return;

    const interval = setInterval(sync, syncInterval);
    return () => clearInterval(interval);
  }, [autoSync, syncInterval, sync]);

  return { sync, syncing, lastSync, error };
}

/**
 * Hook for comparing local (DuckDB) vs remote (ClickHouse) query results
 */
export function useDuckDBComparison<T = Record<string, unknown>>(
  localQuery: string | null,
  remoteFetch: () => Promise<T[]>,
  options: {
    skip?: boolean;
  } = {}
) {
  const [localData, setLocalData] = useState<T[] | null>(null);
  const [remoteData, setRemoteData] = useState<T[] | null>(null);
  const [localTime, setLocalTime] = useState<number>(0);
  const [remoteTime, setRemoteTime] = useState<number>(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const { skip = false } = options;

  const compare = useCallback(async () => {
    if (!localQuery || skip) return;

    setLoading(true);
    setError(null);

    try {
      // Execute local query
      const localStart = performance.now();
      const client = getDuckDBClient();
      const localResult = await client.query<T>(localQuery);
      setLocalTime(performance.now() - localStart);
      setLocalData(localResult.rows);

      // Execute remote query
      const remoteStart = performance.now();
      const remote = await remoteFetch();
      setRemoteTime(performance.now() - remoteStart);
      setRemoteData(remote);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [localQuery, remoteFetch, skip]);

  useEffect(() => {
    compare();
  }, [compare]);

  const speedup = remoteTime > 0 ? remoteTime / localTime : 0;

  return {
    localData,
    remoteData,
    localTime,
    remoteTime,
    speedup,
    loading,
    error,
    refetch: compare,
  };
}
