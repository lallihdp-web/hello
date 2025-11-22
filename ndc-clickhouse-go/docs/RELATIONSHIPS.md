# Relationships Guide

This guide explains how to set up and use relationships between tables in the NDC ClickHouse connector.

## Overview

Relationships allow you to traverse between related tables in GraphQL queries, similar to SQL JOINs but with a more intuitive interface.

## Relationship Types

### Object Relationships (Many-to-One)

An object relationship returns a single related object. Use this when a table has a foreign key to another table.

**Example:** An `order` belongs to a `user`

```json
{
  "name": "user",
  "type": "object",
  "source_table": "orders",
  "target_table": "users",
  "column_mapping": {
    "user_id": "id"
  }
}
```

**GraphQL Query:**
```graphql
query {
  orders {
    id
    total
    user {
      name
      email
    }
  }
}
```

### Array Relationships (One-to-Many)

An array relationship returns multiple related objects. Use this for the reverse direction of a foreign key.

**Example:** A `user` has many `orders`

```json
{
  "name": "orders",
  "type": "array",
  "source_table": "users",
  "target_table": "orders",
  "column_mapping": {
    "id": "user_id"
  }
}
```

**GraphQL Query:**
```graphql
query {
  users {
    id
    name
    orders {
      id
      total
      order_date
    }
  }
}
```

## Configuration

### Full Relationships Configuration

```json
{
  "relationships": {
    "auto_detect": true,
    "foreign_key_pattern": "{table}_id",
    "relationships": [
      {
        "name": "author",
        "type": "object",
        "source_table": "posts",
        "target_table": "users",
        "column_mapping": {
          "author_id": "id"
        },
        "description": "The user who wrote this post"
      },
      {
        "name": "posts",
        "type": "array",
        "source_table": "users",
        "target_table": "posts",
        "column_mapping": {
          "id": "author_id"
        },
        "description": "Posts written by this user"
      },
      {
        "name": "category",
        "type": "object",
        "source_table": "products",
        "target_table": "categories",
        "column_mapping": {
          "category_id": "id"
        }
      },
      {
        "name": "products",
        "type": "array",
        "source_table": "categories",
        "target_table": "products",
        "column_mapping": {
          "id": "category_id"
        }
      }
    ]
  }
}
```

### Relationship Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `name` | string | Yes | GraphQL field name for the relationship |
| `type` | string | Yes | `"object"` or `"array"` |
| `source_table` | string | Yes | Table where the relationship originates |
| `target_table` | string | Yes | Table being referenced |
| `column_mapping` | object | Yes | Source column → target column mapping |
| `description` | string | No | GraphQL description |

## Auto-Detection

The connector can automatically detect relationships based on naming conventions:

```json
{
  "relationships": {
    "auto_detect": true,
    "foreign_key_pattern": "{table}_id"
  }
}
```

### How Auto-Detection Works

1. Scans all tables for columns matching patterns like `{table}_id`, `{table}Id`, `{table}_uuid`
2. Creates both object and array relationships automatically
3. Uses singular/plural forms for naming (e.g., `user` and `orders`)

### Detected Patterns

| Column Name | Detected Target | Relationship Name |
|-------------|-----------------|-------------------|
| `user_id` | `users` | `user` (object) |
| `userId` | `users` | `user` (object) |
| `category_id` | `categories` | `category` (object) |
| `author_id` | `authors` | `author` (object) |

## Complex Relationships

### Composite Keys

For tables with composite primary/foreign keys:

```json
{
  "name": "order_items",
  "type": "array",
  "source_table": "orders",
  "target_table": "order_items",
  "column_mapping": {
    "id": "order_id",
    "version": "order_version"
  }
}
```

### Self-Referential Relationships

Tables that reference themselves:

```json
{
  "relationships": {
    "relationships": [
      {
        "name": "parent",
        "type": "object",
        "source_table": "categories",
        "target_table": "categories",
        "column_mapping": {
          "parent_id": "id"
        }
      },
      {
        "name": "children",
        "type": "array",
        "source_table": "categories",
        "target_table": "categories",
        "column_mapping": {
          "id": "parent_id"
        }
      }
    ]
  }
}
```

**GraphQL Query:**
```graphql
query {
  categories {
    id
    name
    parent {
      name
    }
    children {
      name
    }
  }
}
```

### Many-to-Many Relationships

For many-to-many relationships, you need a junction table:

**Tables:**
- `users`
- `roles`
- `user_roles` (junction table)

```json
{
  "relationships": {
    "relationships": [
      {
        "name": "user_roles",
        "type": "array",
        "source_table": "users",
        "target_table": "user_roles",
        "column_mapping": { "id": "user_id" }
      },
      {
        "name": "user",
        "type": "object",
        "source_table": "user_roles",
        "target_table": "users",
        "column_mapping": { "user_id": "id" }
      },
      {
        "name": "role",
        "type": "object",
        "source_table": "user_roles",
        "target_table": "roles",
        "column_mapping": { "role_id": "id" }
      }
    ]
  }
}
```

**GraphQL Query:**
```graphql
query {
  users {
    id
    name
    user_roles {
      role {
        name
        permissions
      }
    }
  }
}
```

## Filtering Through Relationships

You can filter parent records based on related data:

```graphql
query {
  users(where: { orders: { total: { _gt: 100 } } }) {
    id
    name
  }
}
```

## Ordering by Related Data

Order by aggregations on related data:

```graphql
query {
  users(order_by: { orders_aggregate: { count: desc } }) {
    id
    name
    orders_aggregate {
      aggregate {
        count
      }
    }
  }
}
```

## Best Practices

1. **Use meaningful names**: Name relationships after what they represent, not the table name
2. **Document relationships**: Add descriptions for clarity
3. **Consider performance**: Deep nesting can impact query performance
4. **Use auto-detection**: Start with auto-detection, then customize as needed
5. **Be consistent**: Use the same naming conventions throughout

## Troubleshooting

### Relationship Not Appearing

1. Check that both tables exist in the configuration
2. Verify column names in `column_mapping` match exactly
3. Ensure the relationship configuration is valid JSON

### Query Performance

For slow relationship queries:
- Add appropriate indexes in ClickHouse
- Consider using native queries for complex joins
- Limit the depth of nested queries
