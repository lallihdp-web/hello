import { ApolloClient, InMemoryCache, HttpLink, from, ApolloLink } from '@apollo/client';
import { onError } from '@apollo/client/link/error';

// GraphQL endpoint from environment variables
const GRAPHQL_ENDPOINT = process.env.NEXT_PUBLIC_GRAPHQL_ENDPOINT || 'http://localhost:8080/graphql';

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

// HTTP link
const httpLink = new HttpLink({
  uri: GRAPHQL_ENDPOINT,
});

// Create Apollo Client instance
export const apolloClient = new ApolloClient({
  link: from([errorLink, authLink, httpLink]),
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
    link: from([errorLink, authLink, httpLink]),
    cache: new InMemoryCache(),
  });
}

export default apolloClient;
