'use client';

import { useQuery, gql } from '@apollo/client';

// Simple query to test connection and get schema info
const GET_SCHEMA_INFO = gql`
  query GetSchemaInfo {
    __schema {
      queryType {
        fields {
          name
          description
        }
      }
    }
  }
`;

export default function Dashboard() {
  const { data, loading, error } = useQuery(GET_SCHEMA_INFO);

  return (
    <div className="px-4 py-6 sm:px-0">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Dashboard</h1>
        <p className="mt-2 text-gray-600">
          Connected to ClickHouse via GraphQL NDC Connector
        </p>
      </div>

      {/* Connection Status */}
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3 mb-8">
        <div className="card">
          <div className="flex items-center">
            <div className={`flex-shrink-0 rounded-md p-3 ${error ? 'bg-red-100' : 'bg-green-100'}`}>
              <svg
                className={`h-6 w-6 ${error ? 'text-red-600' : 'text-green-600'}`}
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                {error ? (
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                ) : (
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                )}
              </svg>
            </div>
            <div className="ml-4">
              <p className="text-sm font-medium text-gray-500">Connection Status</p>
              <p className={`text-lg font-semibold ${error ? 'text-red-600' : 'text-green-600'}`}>
                {loading ? 'Connecting...' : error ? 'Disconnected' : 'Connected'}
              </p>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center">
            <div className="flex-shrink-0 bg-blue-100 rounded-md p-3">
              <svg className="h-6 w-6 text-blue-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
              </svg>
            </div>
            <div className="ml-4">
              <p className="text-sm font-medium text-gray-500">Available Collections</p>
              <p className="text-lg font-semibold text-gray-900">
                {loading ? '...' : data?.__schema?.queryType?.fields?.length || 0}
              </p>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center">
            <div className="flex-shrink-0 bg-purple-100 rounded-md p-3">
              <svg className="h-6 w-6 text-purple-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <div className="ml-4">
              <p className="text-sm font-medium text-gray-500">GraphQL Endpoint</p>
              <p className="text-sm font-semibold text-gray-900 truncate max-w-[200px]">
                {process.env.NEXT_PUBLIC_GRAPHQL_ENDPOINT || 'localhost:8080'}
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Available Collections */}
      <div className="card">
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Available Collections</h2>

        {loading && (
          <div className="flex justify-center py-8">
            <div className="spinner"></div>
          </div>
        )}

        {error && (
          <div className="bg-red-50 border border-red-200 rounded-md p-4">
            <p className="text-red-800">
              Failed to connect to GraphQL endpoint. Make sure the ClickHouse NDC connector is running.
            </p>
            <p className="text-red-600 text-sm mt-2">{error.message}</p>
          </div>
        )}

        {data && (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Collection Name</th>
                  <th>Description</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {data.__schema.queryType.fields
                  .filter((field: { name: string }) => !field.name.startsWith('__'))
                  .map((field: { name: string; description?: string }) => (
                    <tr key={field.name}>
                      <td className="font-medium">{field.name}</td>
                      <td className="text-gray-500">{field.description || '-'}</td>
                      <td>
                        <a
                          href={`/explorer?collection=${field.name}`}
                          className="text-blue-600 hover:text-blue-800"
                        >
                          Explore
                        </a>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Quick Start Guide */}
      <div className="card mt-6">
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Quick Start</h2>
        <div className="prose prose-sm max-w-none">
          <p className="text-gray-600 mb-4">
            This Next.js application connects to your ClickHouse database via the GraphQL NDC connector.
          </p>
          <div className="code-block">
            <pre>{`# Start the ClickHouse NDC connector
cd ndc-clickhouse-go
./ndc-clickhouse serve --config config.json

# Start this Next.js app
cd nextjs-clickhouse-app
npm install
npm run dev`}</pre>
          </div>
        </div>
      </div>
    </div>
  );
}
