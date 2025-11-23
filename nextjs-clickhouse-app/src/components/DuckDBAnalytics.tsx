'use client';

/**
 * DuckDB Analytics Dashboard Component
 *
 * Demonstrates client-side analytics using DuckDB-WASM:
 * - Load data from server or files
 * - Execute queries locally
 * - Compare local vs remote performance
 * - View cache statistics
 */

import React, { useState, useCallback } from 'react';
import {
  useDuckDB,
  useDuckDBQuery,
  useDuckDBLoader,
  useDuckDBStats,
  useDuckDBTables,
  useDuckDBAggregation,
} from '@/lib/duckdb';

interface AnalyticsProps {
  /** Server endpoint to fetch data for caching */
  dataEndpoint?: string;
}

export function DuckDBAnalytics({ dataEndpoint }: AnalyticsProps) {
  const { initialized, loading: initLoading, error: initError } = useDuckDB();
  const { loadData, loading: loadLoading, error: loadError } = useDuckDBLoader();
  const { stats, refresh: refreshStats } = useDuckDBStats();
  const { tables, dropTable, clearAll } = useDuckDBTables();

  const [sqlQuery, setSqlQuery] = useState('');
  const [activeTable, setActiveTable] = useState<string | null>(null);

  // Query hook (only runs when sqlQuery changes and is not empty)
  const { data: queryResult, loading: queryLoading, error: queryError } = useDuckDBQuery(
    sqlQuery || null,
    { skip: !sqlQuery }
  );

  // Aggregation example
  const { data: aggResult } = useDuckDBAggregation(
    activeTable,
    {
      aggregations: [
        { column: '*', function: 'count', alias: 'total_rows' },
      ],
      skip: !activeTable,
    }
  );

  // Load sample data
  const handleLoadSampleData = useCallback(async () => {
    const sampleData = [
      { id: 1, name: 'Alice', department: 'Engineering', salary: 100000, hire_date: '2020-01-15' },
      { id: 2, name: 'Bob', department: 'Sales', salary: 80000, hire_date: '2019-06-20' },
      { id: 3, name: 'Charlie', department: 'Engineering', salary: 120000, hire_date: '2018-03-10' },
      { id: 4, name: 'Diana', department: 'Marketing', salary: 90000, hire_date: '2021-08-05' },
      { id: 5, name: 'Eve', department: 'Engineering', salary: 110000, hire_date: '2020-11-30' },
      { id: 6, name: 'Frank', department: 'Sales', salary: 85000, hire_date: '2019-04-15' },
      { id: 7, name: 'Grace', department: 'Marketing', salary: 95000, hire_date: '2022-01-20' },
      { id: 8, name: 'Henry', department: 'Engineering', salary: 130000, hire_date: '2017-09-10' },
    ];

    await loadData('employees', sampleData);
    setActiveTable('employees');
    refreshStats();
  }, [loadData, refreshStats]);

  // Execute custom query
  const handleExecuteQuery = useCallback(() => {
    // Query will be executed by the hook when sqlQuery changes
  }, []);

  // Sample queries
  const sampleQueries = [
    'SELECT * FROM employees LIMIT 10',
    'SELECT department, COUNT(*) as count, AVG(salary) as avg_salary FROM employees GROUP BY department',
    "SELECT * FROM employees WHERE department = 'Engineering' ORDER BY salary DESC",
    'SELECT name, salary FROM employees WHERE salary > 100000',
  ];

  if (initLoading) {
    return (
      <div className="p-6 bg-gray-100 rounded-lg">
        <div className="animate-pulse flex items-center gap-2">
          <div className="w-4 h-4 bg-blue-500 rounded-full animate-bounce"></div>
          <span>Initializing DuckDB-WASM...</span>
        </div>
      </div>
    );
  }

  if (initError) {
    return (
      <div className="p-6 bg-red-100 rounded-lg text-red-700">
        <h3 className="font-bold">DuckDB Initialization Error</h3>
        <p>{initError.message}</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="bg-gradient-to-r from-blue-600 to-purple-600 text-white p-6 rounded-lg">
        <h2 className="text-2xl font-bold mb-2">DuckDB Client-Side Analytics</h2>
        <p className="opacity-90">
          Run SQL queries directly in your browser using DuckDB-WASM.
          No server round-trips for cached data.
        </p>
        <div className="mt-4 flex items-center gap-4 text-sm">
          <span className={`px-2 py-1 rounded ${initialized ? 'bg-green-500' : 'bg-gray-500'}`}>
            {initialized ? 'Initialized' : 'Not Initialized'}
          </span>
          {stats && (
            <>
              <span className="px-2 py-1 bg-blue-500 rounded">
                {stats.tables.length} Tables
              </span>
              <span className="px-2 py-1 bg-purple-500 rounded">
                {stats.totalRows.toLocaleString()} Rows
              </span>
              <span className="px-2 py-1 bg-indigo-500 rounded">
                {stats.queryCount} Queries
              </span>
            </>
          )}
        </div>
      </div>

      {/* Data Loading Section */}
      <div className="bg-white p-6 rounded-lg shadow">
        <h3 className="text-lg font-semibold mb-4">Load Data</h3>
        <div className="flex gap-4">
          <button
            onClick={handleLoadSampleData}
            disabled={loadLoading}
            className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50"
          >
            {loadLoading ? 'Loading...' : 'Load Sample Data'}
          </button>
          <button
            onClick={clearAll}
            className="px-4 py-2 bg-red-500 text-white rounded hover:bg-red-600"
          >
            Clear All Data
          </button>
        </div>
        {loadError && (
          <p className="mt-2 text-red-600">{loadError.message}</p>
        )}
      </div>

      {/* Cached Tables */}
      {tables.length > 0 && (
        <div className="bg-white p-6 rounded-lg shadow">
          <h3 className="text-lg font-semibold mb-4">Cached Tables</h3>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Table</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Rows</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Columns</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Last Updated</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {tables.map((table) => (
                  <tr key={table.name} className={activeTable === table.name ? 'bg-blue-50' : ''}>
                    <td className="px-4 py-2 font-mono text-sm">{table.name}</td>
                    <td className="px-4 py-2 text-sm">{table.rowCount.toLocaleString()}</td>
                    <td className="px-4 py-2 text-sm text-gray-500">
                      {table.columns.slice(0, 3).join(', ')}
                      {table.columns.length > 3 && `... +${table.columns.length - 3}`}
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-500">
                      {table.lastUpdated.toLocaleTimeString()}
                    </td>
                    <td className="px-4 py-2">
                      <button
                        onClick={() => setActiveTable(table.name)}
                        className="text-blue-500 hover:text-blue-700 mr-2"
                      >
                        Select
                      </button>
                      <button
                        onClick={() => dropTable(table.name)}
                        className="text-red-500 hover:text-red-700"
                      >
                        Drop
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Query Editor */}
      <div className="bg-white p-6 rounded-lg shadow">
        <h3 className="text-lg font-semibold mb-4">SQL Query</h3>

        {/* Sample Queries */}
        <div className="mb-4">
          <p className="text-sm text-gray-500 mb-2">Sample queries:</p>
          <div className="flex flex-wrap gap-2">
            {sampleQueries.map((query, idx) => (
              <button
                key={idx}
                onClick={() => setSqlQuery(query)}
                className="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 rounded font-mono"
              >
                {query.substring(0, 40)}...
              </button>
            ))}
          </div>
        </div>

        {/* Query Input */}
        <textarea
          value={sqlQuery}
          onChange={(e) => setSqlQuery(e.target.value)}
          placeholder="Enter SQL query..."
          className="w-full h-32 p-3 border border-gray-300 rounded-lg font-mono text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
        />

        <div className="mt-4 flex items-center gap-4">
          <button
            onClick={handleExecuteQuery}
            disabled={queryLoading || !sqlQuery}
            className="px-4 py-2 bg-green-500 text-white rounded hover:bg-green-600 disabled:opacity-50"
          >
            {queryLoading ? 'Executing...' : 'Execute Query'}
          </button>
          {queryResult && (
            <span className="text-sm text-gray-500">
              {queryResult.rowCount} rows in {queryResult.executionTimeMs.toFixed(2)}ms
            </span>
          )}
        </div>

        {queryError && (
          <p className="mt-2 text-red-600">{queryError.message}</p>
        )}
      </div>

      {/* Query Results */}
      {queryResult && queryResult.rows.length > 0 && (
        <div className="bg-white p-6 rounded-lg shadow">
          <h3 className="text-lg font-semibold mb-4">
            Results ({queryResult.rowCount} rows)
          </h3>
          <div className="overflow-x-auto max-h-96">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50 sticky top-0">
                <tr>
                  {queryResult.columns.map((col) => (
                    <th
                      key={col}
                      className="px-4 py-2 text-left text-sm font-medium text-gray-500"
                    >
                      {col}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {queryResult.rows.slice(0, 100).map((row, idx) => (
                  <tr key={idx} className="hover:bg-gray-50">
                    {queryResult.columns.map((col) => (
                      <td key={col} className="px-4 py-2 text-sm">
                        {String((row as Record<string, unknown>)[col] ?? '')}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
            {queryResult.rowCount > 100 && (
              <p className="mt-2 text-sm text-gray-500 text-center">
                Showing first 100 of {queryResult.rowCount} rows
              </p>
            )}
          </div>
        </div>
      )}

      {/* Aggregation Stats */}
      {aggResult && activeTable && (
        <div className="bg-white p-6 rounded-lg shadow">
          <h3 className="text-lg font-semibold mb-4">
            Table Stats: {activeTable}
          </h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="p-4 bg-blue-50 rounded-lg">
              <p className="text-sm text-gray-500">Total Rows</p>
              <p className="text-2xl font-bold text-blue-600">
                {(aggResult.rows[0] as Record<string, unknown>)?.total_rows?.toLocaleString() ?? 0}
              </p>
            </div>
            <div className="p-4 bg-green-50 rounded-lg">
              <p className="text-sm text-gray-500">Query Time</p>
              <p className="text-2xl font-bold text-green-600">
                {aggResult.executionTimeMs.toFixed(2)}ms
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Performance Comparison Info */}
      <div className="bg-gray-50 p-6 rounded-lg">
        <h3 className="text-lg font-semibold mb-4">How It Works</h3>
        <div className="grid md:grid-cols-3 gap-4">
          <div className="p-4 bg-white rounded-lg shadow-sm">
            <h4 className="font-semibold text-blue-600">1. Load Data</h4>
            <p className="text-sm text-gray-600 mt-2">
              Data from ClickHouse is cached locally in DuckDB-WASM.
              Supports JSON, CSV, and Parquet formats.
            </p>
          </div>
          <div className="p-4 bg-white rounded-lg shadow-sm">
            <h4 className="font-semibold text-green-600">2. Query Locally</h4>
            <p className="text-sm text-gray-600 mt-2">
              Queries run entirely in your browser.
              No network latency for cached data.
            </p>
          </div>
          <div className="p-4 bg-white rounded-lg shadow-sm">
            <h4 className="font-semibold text-purple-600">3. Fast Analytics</h4>
            <p className="text-sm text-gray-600 mt-2">
              Complex aggregations and filters execute in milliseconds.
              Works offline once data is loaded.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

export default DuckDBAnalytics;
