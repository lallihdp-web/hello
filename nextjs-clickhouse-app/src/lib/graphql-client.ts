/**
 * Lightweight GraphQL client for direct fetch operations
 * Use this for server-side data fetching or when Apollo is not needed
 */

import { getQueryLogger } from './logger';

const GRAPHQL_ENDPOINT = process.env.NEXT_PUBLIC_GRAPHQL_ENDPOINT || 'http://localhost:8080/graphql';

// Initialize the query logger
const queryLogger = getQueryLogger();

export interface GraphQLResponse<T> {
  data?: T;
  errors?: Array<{
    message: string;
    locations?: Array<{ line: number; column: number }>;
    path?: string[];
  }>;
}

export interface GraphQLRequestOptions {
  variables?: Record<string, unknown>;
  headers?: Record<string, string>;
  role?: string;
  userId?: string;
}

/**
 * Execute a GraphQL query or mutation
 */
export async function graphqlRequest<T = unknown>(
  query: string,
  options: GraphQLRequestOptions = {}
): Promise<GraphQLResponse<T>> {
  const { variables, headers = {}, role = 'anonymous', userId } = options;
  const stopTimer = queryLogger.startTimer();

  const requestHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Hasura-Role': role,
    ...headers,
  };

  if (userId) {
    requestHeaders['X-Hasura-User-Id'] = userId;
  }

  const apiKey = process.env.NEXT_PUBLIC_API_KEY;
  if (apiKey) {
    requestHeaders['X-API-Key'] = apiKey;
  }

  let result: GraphQLResponse<T>;
  let error: string | undefined;
  let success = true;

  try {
    const response = await fetch(GRAPHQL_ENDPOINT, {
      method: 'POST',
      headers: requestHeaders,
      body: JSON.stringify({
        query,
        variables,
      }),
    });

    if (!response.ok) {
      throw new Error(`GraphQL request failed: ${response.statusText}`);
    }

    result = await response.json();

    if (result.errors && result.errors.length > 0) {
      success = false;
      error = result.errors.map(e => e.message).join(', ');
    }
  } catch (e) {
    success = false;
    error = e instanceof Error ? e.message : 'Unknown error';
    throw e;
  } finally {
    const durationMs = stopTimer();

    queryLogger.log({
      query,
      variables,
      durationMs,
      success,
      error,
      responseData: success ? result! : undefined,
      source: 'fetch',
      metadata: { role, userId },
    });
  }

  return result!;
}

/**
 * Execute a query with automatic error handling
 */
export async function query<T = unknown>(
  queryString: string,
  variables?: Record<string, unknown>,
  options?: Omit<GraphQLRequestOptions, 'variables'>
): Promise<T> {
  const response = await graphqlRequest<T>(queryString, { ...options, variables });

  if (response.errors && response.errors.length > 0) {
    throw new Error(response.errors.map(e => e.message).join(', '));
  }

  if (!response.data) {
    throw new Error('No data returned from GraphQL query');
  }

  return response.data;
}

/**
 * Execute a mutation with automatic error handling
 */
export async function mutate<T = unknown>(
  mutationString: string,
  variables?: Record<string, unknown>,
  options?: Omit<GraphQLRequestOptions, 'variables'>
): Promise<T> {
  return query<T>(mutationString, variables, options);
}

// Export query logger for external access
export { queryLogger };
export { getQueryLogger } from './logger';

export default { graphqlRequest, query, mutate };
