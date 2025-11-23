'use client';

import { useState } from 'react';
import { useLazyQuery, gql } from '@apollo/client';

// Build a dynamic query based on collection name
function buildQuery(collection: string, fields: string[]) {
  const fieldList = fields.length > 0 ? fields.join('\n    ') : 'id';
  return gql`
    query Explore($limit: Int) {
      ${collection}(limit: $limit) {
        ${fieldList}
      }
    }
  `;
}

export default function ExplorerPage() {
  const [collection, setCollection] = useState('');
  const [fields, setFields] = useState('id');
  const [limit, setLimit] = useState(10);
  const [rawQuery, setRawQuery] = useState('');
  const [useRawQuery, setUseRawQuery] = useState(false);

  const [executeQuery, { data, loading, error }] = useLazyQuery(
    useRawQuery && rawQuery
      ? gql(rawQuery)
      : buildQuery(collection || 'users', fields.split(',').map(f => f.trim()).filter(Boolean)),
    { variables: { limit } }
  );

  const handleExecute = () => {
    executeQuery();
  };

  return (
    <div className="px-4 py-6 sm:px-0">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">GraphQL Explorer</h1>
        <p className="mt-2 text-gray-600">
          Execute custom GraphQL queries against your ClickHouse database
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Query Builder */}
        <div className="card">
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Query Builder</h2>

          <div className="space-y-4">
            {/* Mode Toggle */}
            <div className="flex items-center space-x-4">
              <label className="flex items-center">
                <input
                  type="radio"
                  checked={!useRawQuery}
                  onChange={() => setUseRawQuery(false)}
                  className="mr-2"
                />
                Simple Mode
              </label>
              <label className="flex items-center">
                <input
                  type="radio"
                  checked={useRawQuery}
                  onChange={() => setUseRawQuery(true)}
                  className="mr-2"
                />
                Raw Query
              </label>
            </div>

            {!useRawQuery ? (
              <>
                {/* Collection */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Collection Name
                  </label>
                  <input
                    type="text"
                    value={collection}
                    onChange={(e) => setCollection(e.target.value)}
                    placeholder="e.g., users, events, orders"
                    className="input"
                  />
                </div>

                {/* Fields */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Fields (comma-separated)
                  </label>
                  <input
                    type="text"
                    value={fields}
                    onChange={(e) => setFields(e.target.value)}
                    placeholder="e.g., id, name, email"
                    className="input"
                  />
                </div>

                {/* Limit */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Limit
                  </label>
                  <input
                    type="number"
                    value={limit}
                    onChange={(e) => setLimit(parseInt(e.target.value) || 10)}
                    min={1}
                    max={1000}
                    className="input"
                  />
                </div>
              </>
            ) : (
              /* Raw Query */
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  GraphQL Query
                </label>
                <textarea
                  value={rawQuery}
                  onChange={(e) => setRawQuery(e.target.value)}
                  placeholder={`query {
  users(limit: 10) {
    id
    name
  }
}`}
                  rows={10}
                  className="input font-mono text-sm"
                />
              </div>
            )}

            <button
              onClick={handleExecute}
              disabled={loading || (!collection && !useRawQuery) || (useRawQuery && !rawQuery)}
              className="btn btn-primary w-full disabled:opacity-50"
            >
              {loading ? 'Executing...' : 'Execute Query'}
            </button>
          </div>

          {/* Generated Query Preview */}
          {!useRawQuery && collection && (
            <div className="mt-6">
              <h3 className="text-sm font-medium text-gray-700 mb-2">Generated Query</h3>
              <div className="code-block text-xs">
                <pre>{`query Explore($limit: Int) {
  ${collection}(limit: $limit) {
    ${fields.split(',').map(f => f.trim()).filter(Boolean).join('\n    ')}
  }
}`}</pre>
              </div>
            </div>
          )}
        </div>

        {/* Results */}
        <div className="card">
          <h2 className="text-xl font-semibold text-gray-900 mb-4">Results</h2>

          {loading && (
            <div className="flex justify-center py-8">
              <div className="spinner"></div>
            </div>
          )}

          {error && (
            <div className="bg-red-50 border border-red-200 rounded-md p-4">
              <p className="text-red-800 font-medium">Query Error</p>
              <p className="text-red-600 text-sm mt-2">{error.message}</p>
            </div>
          )}

          {data && (
            <div className="code-block overflow-auto max-h-[600px]">
              <pre>{JSON.stringify(data, null, 2)}</pre>
            </div>
          )}

          {!loading && !error && !data && (
            <div className="text-center py-8 text-gray-500">
              <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
              </svg>
              <p className="mt-2">Enter a query and click Execute</p>
            </div>
          )}
        </div>
      </div>

      {/* Help Section */}
      <div className="card mt-6">
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Example Queries</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <h3 className="font-medium text-gray-700 mb-2">Simple Query</h3>
            <div className="code-block text-xs">
              <pre>{`query {
  users(limit: 10) {
    id
    name
    email
  }
}`}</pre>
            </div>
          </div>
          <div>
            <h3 className="font-medium text-gray-700 mb-2">With Filter</h3>
            <div className="code-block text-xs">
              <pre>{`query {
  users(
    where: { name: { _like: "%john%" } }
    limit: 10
  ) {
    id
    name
  }
}`}</pre>
            </div>
          </div>
          <div>
            <h3 className="font-medium text-gray-700 mb-2">Aggregation</h3>
            <div className="code-block text-xs">
              <pre>{`query {
  users_aggregate {
    aggregate {
      count
      avg {
        age
      }
    }
  }
}`}</pre>
            </div>
          </div>
          <div>
            <h3 className="font-medium text-gray-700 mb-2">With Ordering</h3>
            <div className="code-block text-xs">
              <pre>{`query {
  events(
    order_by: { timestamp: desc }
    limit: 100
  ) {
    event_type
    timestamp
  }
}`}</pre>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
