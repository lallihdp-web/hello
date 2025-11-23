# Next.js ClickHouse GraphQL Client

A Next.js application that connects to ClickHouse via the GraphQL NDC connector.

## Features

- **Dashboard**: Overview of connected collections and status
- **Users Page**: Example page for querying user data
- **Events Page**: Analytics events viewer with filtering
- **GraphQL Explorer**: Interactive query builder for custom queries
- **Apollo Client**: Full-featured GraphQL client with caching
- **Lightweight Client**: Fetch-based client for server-side rendering

## Prerequisites

1. ClickHouse database running
2. NDC ClickHouse connector running with GraphQL enabled

## Getting Started

### 1. Start the ClickHouse NDC Connector

```bash
cd ../ndc-clickhouse-go
./ndc-clickhouse serve --config config.json
```

The connector will expose a GraphQL endpoint at `http://localhost:8080/graphql`.

### 2. Configure Environment

Copy the example environment file:

```bash
cp .env.local.example .env.local
```

Edit `.env.local` to set your GraphQL endpoint:

```env
NEXT_PUBLIC_GRAPHQL_ENDPOINT=http://localhost:8080/graphql
NEXT_PUBLIC_API_KEY=your-api-key-here
NEXT_PUBLIC_DEFAULT_ROLE=user
```

### 3. Install Dependencies

```bash
npm install
```

### 4. Run Development Server

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) to view the application.

## Project Structure

```
src/
├── app/                    # Next.js App Router pages
│   ├── layout.tsx         # Root layout with Apollo Provider
│   ├── page.tsx           # Dashboard page
│   ├── users/             # Users page
│   ├── events/            # Events page
│   └── explorer/          # GraphQL explorer
├── components/            # Reusable React components
│   ├── DataTable.tsx      # Sortable, paginated data table
│   └── QueryBuilder.tsx   # Visual query builder
├── graphql/               # GraphQL queries and mutations
│   ├── queries.graphql    # Query definitions
│   └── mutations.graphql  # Mutation definitions
├── hooks/                 # Custom React hooks
│   └── useClickHouseQuery.ts
└── lib/                   # Library utilities
    ├── apollo-client.ts   # Apollo Client configuration
    └── graphql-client.ts  # Lightweight fetch client
```

## GraphQL Client Options

### Apollo Client (Recommended for React)

Full-featured client with caching, subscriptions, and devtools support:

```tsx
import { useQuery, gql } from '@apollo/client';

const GET_USERS = gql`
  query GetUsers {
    users(limit: 10) {
      id
      name
    }
  }
`;

function MyComponent() {
  const { data, loading, error } = useQuery(GET_USERS);
  // ...
}
```

### Lightweight Client (Server-side)

For server components or simple fetch operations:

```tsx
import { query } from '@/lib/graphql-client';

async function getUsers() {
  const data = await query(`
    query {
      users(limit: 10) {
        id
        name
      }
    }
  `);
  return data;
}
```

## Customization

### Adding New Collections

1. Update the GraphQL queries in `src/graphql/queries.graphql`
2. Run codegen to generate types: `npm run codegen`
3. Create a new page in `src/app/[collection]/page.tsx`

### Authentication

The client automatically adds Hasura-style headers for authentication:

- `X-API-Key`: API key from environment
- `X-Hasura-Role`: User role for permissions
- `X-Hasura-User-Id`: User identifier

## Available Scripts

- `npm run dev` - Start development server
- `npm run build` - Build for production
- `npm run start` - Start production server
- `npm run lint` - Run ESLint
- `npm run codegen` - Generate GraphQL types

## Learn More

- [Next.js Documentation](https://nextjs.org/docs)
- [Apollo Client Documentation](https://www.apollographql.com/docs/react/)
- [ClickHouse NDC Connector](../ndc-clickhouse-go/README.md)
