import { useState, useCallback } from 'react';
import { graphqlRequest, GraphQLResponse, GraphQLRequestOptions } from '@/lib/graphql-client';

interface UseClickHouseQueryOptions extends GraphQLRequestOptions {
  onSuccess?: (data: unknown) => void;
  onError?: (error: Error) => void;
}

interface UseClickHouseQueryReturn<T> {
  data: T | null;
  loading: boolean;
  error: Error | null;
  execute: (variables?: Record<string, unknown>) => Promise<void>;
  reset: () => void;
}

/**
 * Custom hook for executing ClickHouse GraphQL queries
 * Uses the lightweight fetch-based client for server-side or simple queries
 */
export function useClickHouseQuery<T = unknown>(
  query: string,
  options: UseClickHouseQueryOptions = {}
): UseClickHouseQueryReturn<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const execute = useCallback(
    async (variables?: Record<string, unknown>) => {
      setLoading(true);
      setError(null);

      try {
        const response: GraphQLResponse<T> = await graphqlRequest<T>(query, {
          ...options,
          variables: { ...options.variables, ...variables },
        });

        if (response.errors && response.errors.length > 0) {
          throw new Error(response.errors.map((e) => e.message).join(', '));
        }

        if (response.data) {
          setData(response.data);
          options.onSuccess?.(response.data);
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error('Unknown error');
        setError(error);
        options.onError?.(error);
      } finally {
        setLoading(false);
      }
    },
    [query, options]
  );

  const reset = useCallback(() => {
    setData(null);
    setError(null);
    setLoading(false);
  }, []);

  return { data, loading, error, execute, reset };
}

/**
 * Hook for lazy loading ClickHouse data
 */
export function useLazyClickHouseQuery<T = unknown>(
  query: string,
  options: UseClickHouseQueryOptions = {}
): [
  (variables?: Record<string, unknown>) => Promise<void>,
  UseClickHouseQueryReturn<T>
] {
  const result = useClickHouseQuery<T>(query, options);
  return [result.execute, result];
}

export default useClickHouseQuery;
