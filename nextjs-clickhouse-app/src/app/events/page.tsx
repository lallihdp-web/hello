'use client';

import { useState } from 'react';
import { useQuery, gql } from '@apollo/client';

// Dynamic query - adjust based on your ClickHouse schema
const GET_EVENTS = gql`
  query GetEvents($limit: Int, $offset: Int) {
    events(limit: $limit, offset: $offset, order_by: { timestamp: desc }) {
      id
      event_type
      user_id
      timestamp
      properties
    }
  }
`;

const GET_EVENTS_COUNT = gql`
  query GetEventsCount {
    events_aggregate {
      aggregate {
        count
      }
    }
  }
`;

interface Event {
  id: string;
  event_type: string;
  user_id: string;
  timestamp: string;
  properties: Record<string, unknown>;
}

export default function EventsPage() {
  const [page, setPage] = useState(0);
  const [eventTypeFilter, setEventTypeFilter] = useState('');
  const limit = 20;

  const { data, loading, error, refetch } = useQuery(GET_EVENTS, {
    variables: { limit, offset: page * limit },
  });

  const { data: countData } = useQuery(GET_EVENTS_COUNT);
  const totalCount = countData?.events_aggregate?.aggregate?.count || 0;
  const totalPages = Math.ceil(totalCount / limit);

  const filteredEvents = eventTypeFilter
    ? data?.events?.filter((e: Event) => e.event_type.includes(eventTypeFilter))
    : data?.events;

  return (
    <div className="px-4 py-6 sm:px-0">
      <div className="mb-8 flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Events</h1>
          <p className="mt-2 text-gray-600">
            Analytics events from ClickHouse
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
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-4 mb-8">
        <div className="card">
          <p className="text-sm font-medium text-gray-500">Total Events</p>
          <p className="text-2xl font-semibold text-gray-900">{totalCount.toLocaleString()}</p>
        </div>
        <div className="card">
          <p className="text-sm font-medium text-gray-500">Showing</p>
          <p className="text-2xl font-semibold text-gray-900">{filteredEvents?.length || 0}</p>
        </div>
        <div className="card col-span-2">
          <p className="text-sm font-medium text-gray-500 mb-2">Filter by Event Type</p>
          <input
            type="text"
            value={eventTypeFilter}
            onChange={(e) => setEventTypeFilter(e.target.value)}
            placeholder="e.g., page_view, click"
            className="input"
          />
        </div>
      </div>

      {/* Events Table */}
      <div className="card">
        {loading && (
          <div className="flex justify-center py-8">
            <div className="spinner"></div>
          </div>
        )}

        {error && (
          <div className="bg-yellow-50 border border-yellow-200 rounded-md p-4">
            <p className="text-yellow-800">
              Note: The &apos;events&apos; collection may not exist in your schema.
              Update the query to match your ClickHouse tables.
            </p>
            <p className="text-yellow-600 text-sm mt-2">{error.message}</p>
          </div>
        )}

        {filteredEvents && (
          <>
            <div className="overflow-x-auto">
              <table className="table">
                <thead>
                  <tr>
                    <th>Event ID</th>
                    <th>Type</th>
                    <th>User ID</th>
                    <th>Timestamp</th>
                    <th>Properties</th>
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {filteredEvents.map((event: Event) => (
                    <tr key={event.id}>
                      <td className="font-mono text-xs">{event.id}</td>
                      <td>
                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                          {event.event_type}
                        </span>
                      </td>
                      <td className="text-gray-500">{event.user_id}</td>
                      <td className="text-gray-500 text-sm">
                        {new Date(event.timestamp).toLocaleString()}
                      </td>
                      <td className="text-xs font-mono text-gray-500 max-w-xs truncate">
                        {JSON.stringify(event.properties)}
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
          <pre>{`query GetEvents($limit: Int, $offset: Int) {
  events(
    limit: $limit,
    offset: $offset,
    order_by: { timestamp: desc }
  ) {
    id
    event_type
    user_id
    timestamp
    properties
  }
}`}</pre>
        </div>
      </div>
    </div>
  );
}
