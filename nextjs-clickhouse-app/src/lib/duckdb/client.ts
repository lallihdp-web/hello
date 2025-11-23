/**
 * DuckDB-WASM Client for Browser-Side Analytics
 *
 * This module provides client-side analytics capabilities using DuckDB-WASM,
 * allowing for fast local queries on cached data without server round-trips.
 */

import * as duckdb from '@duckdb/duckdb-wasm';
import { Table } from 'apache-arrow';

// DuckDB bundle configuration
const DUCKDB_BUNDLES: duckdb.DuckDBBundles = {
  mvp: {
    mainModule: 'https://cdn.jsdelivr.net/npm/@duckdb/duckdb-wasm@1.28.0/dist/duckdb-mvp.wasm',
    mainWorker: 'https://cdn.jsdelivr.net/npm/@duckdb/duckdb-wasm@1.28.0/dist/duckdb-browser-mvp.worker.js',
  },
  eh: {
    mainModule: 'https://cdn.jsdelivr.net/npm/@duckdb/duckdb-wasm@1.28.0/dist/duckdb-eh.wasm',
    mainWorker: 'https://cdn.jsdelivr.net/npm/@duckdb/duckdb-wasm@1.28.0/dist/duckdb-browser-eh.worker.js',
  },
};

export interface DuckDBConfig {
  /** Maximum memory usage in MB */
  maxMemoryMB?: number;
  /** Enable query logging */
  enableLogging?: boolean;
}

export interface QueryResult<T = Record<string, unknown>> {
  rows: T[];
  columns: string[];
  rowCount: number;
  executionTimeMs: number;
}

export interface CachedTable {
  name: string;
  rowCount: number;
  columns: string[];
  lastUpdated: Date;
  sizeBytes: number;
}

export interface DuckDBStats {
  initialized: boolean;
  tables: CachedTable[];
  totalRows: number;
  totalSizeBytes: number;
  queryCount: number;
  avgQueryTimeMs: number;
}

/**
 * DuckDB-WASM Client for client-side analytics
 */
class DuckDBClient {
  private db: duckdb.AsyncDuckDB | null = null;
  private conn: duckdb.AsyncDuckDBConnection | null = null;
  private initialized = false;
  private initializing = false;
  private config: DuckDBConfig;
  private stats = {
    queryCount: 0,
    totalQueryTimeMs: 0,
  };

  constructor(config: DuckDBConfig = {}) {
    this.config = {
      maxMemoryMB: config.maxMemoryMB ?? 512,
      enableLogging: config.enableLogging ?? false,
    };
  }

  /**
   * Initialize DuckDB-WASM
   */
  async initialize(): Promise<void> {
    if (this.initialized) return;
    if (this.initializing) {
      // Wait for existing initialization
      while (this.initializing) {
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      return;
    }

    this.initializing = true;

    try {
      // Select the best bundle for the browser
      const bundle = await duckdb.selectBundle(DUCKDB_BUNDLES);

      // Instantiate the worker
      const worker = new Worker(bundle.mainWorker!);
      const logger = this.config.enableLogging
        ? new duckdb.ConsoleLogger()
        : new duckdb.VoidLogger();

      // Instantiate DuckDB
      this.db = new duckdb.AsyncDuckDB(logger, worker);
      await this.db.instantiate(bundle.mainModule);

      // Create connection
      this.conn = await this.db.connect();

      // Set memory limit
      await this.conn.query(`SET memory_limit = '${this.config.maxMemoryMB}MB'`);

      // Create metadata table
      await this.conn.query(`
        CREATE TABLE IF NOT EXISTS _cache_metadata (
          table_name VARCHAR PRIMARY KEY,
          row_count BIGINT,
          last_updated TIMESTAMP,
          size_bytes BIGINT
        )
      `);

      this.initialized = true;
      console.log('[DuckDB] Initialized successfully');
    } catch (error) {
      console.error('[DuckDB] Initialization failed:', error);
      throw error;
    } finally {
      this.initializing = false;
    }
  }

  /**
   * Ensure DuckDB is initialized
   */
  private async ensureInitialized(): Promise<void> {
    if (!this.initialized) {
      await this.initialize();
    }
  }

  /**
   * Execute a SQL query
   */
  async query<T = Record<string, unknown>>(sql: string): Promise<QueryResult<T>> {
    await this.ensureInitialized();

    const startTime = performance.now();

    try {
      const result = await this.conn!.query(sql);
      const executionTimeMs = performance.now() - startTime;

      // Update stats
      this.stats.queryCount++;
      this.stats.totalQueryTimeMs += executionTimeMs;

      // Convert Arrow table to JSON
      const rows = this.arrowToJson<T>(result);

      return {
        rows,
        columns: result.schema.fields.map(f => f.name),
        rowCount: rows.length,
        executionTimeMs,
      };
    } catch (error) {
      console.error('[DuckDB] Query failed:', sql, error);
      throw error;
    }
  }

  /**
   * Load data from JSON into a table
   */
  async loadData(tableName: string, data: Record<string, unknown>[]): Promise<void> {
    await this.ensureInitialized();

    if (data.length === 0) return;

    // Drop existing table
    await this.conn!.query(`DROP TABLE IF EXISTS ${tableName}`);

    // Create table from JSON
    const jsonStr = JSON.stringify(data);
    await this.db!.registerFileText(`${tableName}.json`, jsonStr);
    await this.conn!.query(`
      CREATE TABLE ${tableName} AS
      SELECT * FROM read_json_auto('${tableName}.json')
    `);

    // Get size estimate
    const sizeResult = await this.conn!.query(`
      SELECT COUNT(*) as cnt FROM ${tableName}
    `);
    const rowCount = this.arrowToJson<{ cnt: number }>(sizeResult)[0]?.cnt ?? 0;

    // Update metadata
    await this.conn!.query(`
      INSERT OR REPLACE INTO _cache_metadata (table_name, row_count, last_updated, size_bytes)
      VALUES ('${tableName}', ${rowCount}, NOW(), ${jsonStr.length})
    `);

    console.log(`[DuckDB] Loaded ${rowCount} rows into ${tableName}`);
  }

  /**
   * Load data from a Parquet URL
   */
  async loadFromParquet(tableName: string, url: string): Promise<void> {
    await this.ensureInitialized();

    // Drop existing table
    await this.conn!.query(`DROP TABLE IF EXISTS ${tableName}`);

    // Create table from Parquet
    await this.conn!.query(`
      CREATE TABLE ${tableName} AS
      SELECT * FROM read_parquet('${url}')
    `);

    // Get row count
    const sizeResult = await this.conn!.query(`
      SELECT COUNT(*) as cnt FROM ${tableName}
    `);
    const rowCount = this.arrowToJson<{ cnt: number }>(sizeResult)[0]?.cnt ?? 0;

    // Update metadata
    await this.conn!.query(`
      INSERT OR REPLACE INTO _cache_metadata (table_name, row_count, last_updated, size_bytes)
      VALUES ('${tableName}', ${rowCount}, NOW(), 0)
    `);

    console.log(`[DuckDB] Loaded ${rowCount} rows from Parquet into ${tableName}`);
  }

  /**
   * Load data from CSV
   */
  async loadFromCSV(tableName: string, csvContent: string): Promise<void> {
    await this.ensureInitialized();

    // Drop existing table
    await this.conn!.query(`DROP TABLE IF EXISTS ${tableName}`);

    // Register CSV content
    await this.db!.registerFileText(`${tableName}.csv`, csvContent);

    // Create table from CSV
    await this.conn!.query(`
      CREATE TABLE ${tableName} AS
      SELECT * FROM read_csv_auto('${tableName}.csv')
    `);

    // Get row count
    const sizeResult = await this.conn!.query(`
      SELECT COUNT(*) as cnt FROM ${tableName}
    `);
    const rowCount = this.arrowToJson<{ cnt: number }>(sizeResult)[0]?.cnt ?? 0;

    // Update metadata
    await this.conn!.query(`
      INSERT OR REPLACE INTO _cache_metadata (table_name, row_count, last_updated, size_bytes)
      VALUES ('${tableName}', ${rowCount}, NOW(), ${csvContent.length})
    `);

    console.log(`[DuckDB] Loaded ${rowCount} rows from CSV into ${tableName}`);
  }

  /**
   * Check if a table exists
   */
  async tableExists(tableName: string): Promise<boolean> {
    await this.ensureInitialized();

    const result = await this.conn!.query(`
      SELECT COUNT(*) as cnt FROM information_schema.tables
      WHERE table_name = '${tableName}'
    `);
    const rows = this.arrowToJson<{ cnt: number }>(result);
    return (rows[0]?.cnt ?? 0) > 0;
  }

  /**
   * Get list of cached tables
   */
  async getTables(): Promise<CachedTable[]> {
    await this.ensureInitialized();

    const result = await this.conn!.query(`
      SELECT table_name, row_count, last_updated, size_bytes
      FROM _cache_metadata
      ORDER BY table_name
    `);

    const rows = this.arrowToJson<{
      table_name: string;
      row_count: number;
      last_updated: Date;
      size_bytes: number;
    }>(result);

    const tables: CachedTable[] = [];

    for (const row of rows) {
      // Get column info
      const schemaResult = await this.conn!.query(`
        SELECT column_name FROM information_schema.columns
        WHERE table_name = '${row.table_name}'
      `);
      const columns = this.arrowToJson<{ column_name: string }>(schemaResult)
        .map(r => r.column_name);

      tables.push({
        name: row.table_name,
        rowCount: row.row_count,
        columns,
        lastUpdated: new Date(row.last_updated),
        sizeBytes: row.size_bytes,
      });
    }

    return tables;
  }

  /**
   * Drop a table
   */
  async dropTable(tableName: string): Promise<void> {
    await this.ensureInitialized();

    await this.conn!.query(`DROP TABLE IF EXISTS ${tableName}`);
    await this.conn!.query(`
      DELETE FROM _cache_metadata WHERE table_name = '${tableName}'
    `);

    console.log(`[DuckDB] Dropped table ${tableName}`);
  }

  /**
   * Clear all cached data
   */
  async clearAll(): Promise<void> {
    await this.ensureInitialized();

    const tables = await this.getTables();
    for (const table of tables) {
      if (!table.name.startsWith('_')) {
        await this.dropTable(table.name);
      }
    }

    console.log('[DuckDB] Cleared all cached data');
  }

  /**
   * Get statistics
   */
  async getStats(): Promise<DuckDBStats> {
    await this.ensureInitialized();

    const tables = await this.getTables();
    const totalRows = tables.reduce((sum, t) => sum + t.rowCount, 0);
    const totalSizeBytes = tables.reduce((sum, t) => sum + t.sizeBytes, 0);

    return {
      initialized: this.initialized,
      tables,
      totalRows,
      totalSizeBytes,
      queryCount: this.stats.queryCount,
      avgQueryTimeMs: this.stats.queryCount > 0
        ? this.stats.totalQueryTimeMs / this.stats.queryCount
        : 0,
    };
  }

  /**
   * Export table to JSON
   */
  async exportToJson(tableName: string): Promise<Record<string, unknown>[]> {
    const result = await this.query(`SELECT * FROM ${tableName}`);
    return result.rows;
  }

  /**
   * Run analytics query with automatic aggregation
   */
  async aggregate<T = Record<string, unknown>>(
    tableName: string,
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
    }
  ): Promise<QueryResult<T>> {
    const { groupBy, aggregations, filters, orderBy, limit } = options;

    // Build SELECT clause
    const selectParts: string[] = [];

    if (groupBy) {
      selectParts.push(...groupBy);
    }

    for (const agg of aggregations) {
      const alias = agg.alias ?? `${agg.function}_${agg.column}`;
      selectParts.push(`${agg.function.toUpperCase()}(${agg.column}) AS ${alias}`);
    }

    let sql = `SELECT ${selectParts.join(', ')} FROM ${tableName}`;

    // WHERE clause
    if (filters && Object.keys(filters).length > 0) {
      const conditions = Object.entries(filters).map(([col, val]) => {
        if (typeof val === 'string') {
          return `${col} = '${val}'`;
        }
        return `${col} = ${val}`;
      });
      sql += ` WHERE ${conditions.join(' AND ')}`;
    }

    // GROUP BY clause
    if (groupBy && groupBy.length > 0) {
      sql += ` GROUP BY ${groupBy.join(', ')}`;
    }

    // ORDER BY clause
    if (orderBy) {
      sql += ` ORDER BY ${orderBy}`;
    }

    // LIMIT clause
    if (limit) {
      sql += ` LIMIT ${limit}`;
    }

    return this.query<T>(sql);
  }

  /**
   * Convert Arrow table to JSON
   */
  private arrowToJson<T>(table: Table): T[] {
    const rows: T[] = [];
    const columns = table.schema.fields.map(f => f.name);

    for (let i = 0; i < table.numRows; i++) {
      const row: Record<string, unknown> = {};
      for (const col of columns) {
        const value = table.getChild(col)?.get(i);
        row[col] = value;
      }
      rows.push(row as T);
    }

    return rows;
  }

  /**
   * Close the connection
   */
  async close(): Promise<void> {
    if (this.conn) {
      await this.conn.close();
      this.conn = null;
    }
    if (this.db) {
      await this.db.terminate();
      this.db = null;
    }
    this.initialized = false;
    console.log('[DuckDB] Connection closed');
  }
}

// Singleton instance
let clientInstance: DuckDBClient | null = null;

/**
 * Get or create the DuckDB client instance
 */
export function getDuckDBClient(config?: DuckDBConfig): DuckDBClient {
  if (!clientInstance) {
    clientInstance = new DuckDBClient(config);
  }
  return clientInstance;
}

/**
 * Reset the DuckDB client (for testing)
 */
export async function resetDuckDBClient(): Promise<void> {
  if (clientInstance) {
    await clientInstance.close();
    clientInstance = null;
  }
}

export default DuckDBClient;
