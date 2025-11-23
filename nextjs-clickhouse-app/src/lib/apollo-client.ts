import { ApolloClient, InMemoryCache, HttpLink, from, ApolloLink } from '@apollo/client';
import { onError } from '@apollo/client/link/error';
import { getQueryLogger } from './logger';

// GraphQL endpoint from environment variables
const GRAPHQL_ENDPOINT = process.env.NEXT_PUBLIC_GRAPHQL_ENDPOINT || 'http://localhost:8080/graphql';

// Initialize the query logger
const queryLogger = getQueryLogger();

// Error handling link
const errorLink = onError(({ graphQLErrors, networkError }) => {
  if (graphQLErrors) {
    graphQLErrors.forEach(({ message, locations, path }) => {
      console.error(
        `[GraphQL error]: Message: ${message}, Location: ${locations}, Path: ${path}`
      );
    });
  }
  if (networkError) {
    console.error(`[Network error]: ${networkError}`);
  }
});

// Authentication link - adds headers for Hasura-style auth
const authLink = new ApolloLink((operation, forward) => {
  const apiKey = process.env.NEXT_PUBLIC_API_KEY;
  const role = process.env.NEXT_PUBLIC_DEFAULT_ROLE || 'anonymous';

  operation.setContext(({ headers = {} }) => ({
    headers: {
      ...headers,
      ...(apiKey && { 'X-API-Key': apiKey }),
      'X-Hasura-Role': role,
    },
  }));

  return forward(operation);
});

// Query logging link - measures execution time and logs slow queries
const loggingLink = new ApolloLink((operation, forward) => {
  const stopTimer = queryLogger.startTimer();
  const query = operation.query.loc?.source.body || '';

  return forward(operation).map((response) => {
    const durationMs = stopTimer();
    const hasErrors = !!(response.errors && response.errors.length > 0);

    queryLogger.log({
      query,
      variables: operation.variables,
      durationMs,
      success: !hasErrors,
      error: hasErrors ? response.errors?.map(e => e.message).join(', ') : undefined,
      responseData: response.data,
      source: 'apollo',
      metadata: {
        operationName: operation.operationName,
        extensions: response.extensions,
      },
    });

    return response;
  });
});

// HTTP link
const httpLink = new HttpLink({
  uri: GRAPHQL_ENDPOINT,
});

// Create Apollo Client instance
export const apolloClient = new ApolloClient({
  link: from([errorLink, authLink, loggingLink, httpLink]),
  cache: new InMemoryCache({
    typePolicies: {
      Query: {
        fields: {
          // Add field policies for pagination if needed
        },
      },
    },
  }),
  defaultOptions: {
    watchQuery: {
      fetchPolicy: 'cache-and-network',
      errorPolicy: 'ignore',
    },
    query: {
      fetchPolicy: 'network-only',
      errorPolicy: 'all',
    },
    mutate: {
      errorPolicy: 'all',
    },
  },
});

// Create client for server-side rendering
export function createApolloClient() {
  return new ApolloClient({
    ssrMode: typeof window === 'undefined',
    link: from([errorLink, authLink, loggingLink, httpLink]),
    cache: new InMemoryCache(),
  });
}

// Export query logger for external access
export { queryLogger };

// Export logger utilities
export { getQueryLogger } from './logger';

export default apolloClient;
