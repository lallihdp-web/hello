'use client';

import { useState } from 'react';
import { useQuery, gql } from '@apollo/client';

// Dynamic query - adjust based on your ClickHouse schema
const GET_USERS = gql`
  query GetUsers($limit: Int, $offset: Int) {
    users(limit: $limit, offset: $offset) {
      id
      name
      email
      created_at
    }
  }
`;

const GET_USERS_COUNT = gql`
  query GetUsersCount {
    users_aggregate {
      aggregate {
        count
      }
    }
  }
`;

interface User {
  id: number;
  name: string;
  email: string;
  created_at: string;
}

export default function UsersPage() {
  const [page, setPage] = useState(0);
  const limit = 10;

  const { data, loading, error, refetch } = useQuery(GET_USERS, {
    variables: { limit, offset: page * limit },
  });

  const { data: countData } = useQuery(GET_USERS_COUNT);
  const totalCount = countData?.users_aggregate?.aggregate?.count || 0;
  const totalPages = Math.ceil(totalCount / limit);

  return (
    <div className="px-4 py-6 sm:px-0">
      <div className="mb-8 flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Users</h1>
          <p className="mt-2 text-gray-600">
            Manage users from your ClickHouse database
          </p>
        </div>
        <button
          onClick={() => refetch()}
          className="btn btn-primary"
        >
          Refresh
        </button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-3 mb-8">
        <div className="card">
          <p className="text-sm font-medium text-gray-500">Total Users</p>
          <p className="text-2xl font-semibold text-gray-900">{totalCount.toLocaleString()}</p>
        </div>
        <div className="card">
          <p className="text-sm font-medium text-gray-500">Current Page</p>
          <p className="text-2xl font-semibold text-gray-900">{page + 1} / {totalPages || 1}</p>
        </div>
        <div className="card">
          <p className="text-sm font-medium text-gray-500">Showing</p>
          <p className="text-2xl font-semibold text-gray-900">{data?.users?.length || 0} records</p>
        </div>
      </div>

      {/* Users Table */}
      <div className="card">
        {loading && (
          <div className="flex justify-center py-8">
            <div className="spinner"></div>
          </div>
        )}

        {error && (
          <div className="bg-yellow-50 border border-yellow-200 rounded-md p-4">
            <p className="text-yellow-800">
              Note: The &apos;users&apos; collection may not exist in your schema.
              Update the query to match your ClickHouse tables.
            </p>
            <p className="text-yellow-600 text-sm mt-2">{error.message}</p>
          </div>
        )}

        {data?.users && (
          <>
            <div className="overflow-x-auto">
              <table className="table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Name</th>
                    <th>Email</th>
                    <th>Created At</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {data.users.map((user: User) => (
                    <tr key={user.id}>
                      <td>{user.id}</td>
                      <td className="font-medium">{user.name}</td>
                      <td className="text-gray-500">{user.email}</td>
                      <td className="text-gray-500">
                        {new Date(user.created_at).toLocaleDateString()}
                      </td>
                      <td>
                        <button className="text-blue-600 hover:text-blue-800">
                          View
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            <div className="mt-4 flex justify-between items-center">
              <button
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={page === 0}
                className="btn btn-secondary disabled:opacity-50"
              >
                Previous
              </button>
              <span className="text-sm text-gray-600">
                Page {page + 1} of {totalPages || 1}
              </span>
              <button
                onClick={() => setPage((p) => p + 1)}
                disabled={page >= totalPages - 1}
                className="btn btn-secondary disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </>
        )}
      </div>

      {/* Sample Query */}
      <div className="card mt-6">
        <h3 className="text-lg font-semibold text-gray-900 mb-4">GraphQL Query</h3>
        <div className="code-block">
          <pre>{`query GetUsers($limit: Int, $offset: Int) {
  users(limit: $limit, offset: $offset) {
    id
    name
    email
    created_at
  }
}`}</pre>
        </div>
      </div>
    </div>
  );
}
