# Permissions Guide

This guide explains how to set up row-level security and access control in the NDC ClickHouse connector.

## Overview

The permissions system allows you to:
- Control which tables and columns each role can access
- Apply row-level security filters
- Set automatic column values on insert
- Limit query results
- Control aggregation access

## Basic Setup

```json
{
  "permissions": {
    "admin_role": "admin",
    "default_role": "anonymous",
    "roles": {
      "admin": { ... },
      "user": { ... },
      "anonymous": { ... }
    }
  }
}
```

### Configuration Options

| Option | Type | Description |
|--------|------|-------------|
| `admin_role` | string | Role with full access (bypasses all checks) |
| `default_role` | string | Role used when none specified |
| `roles` | object | Permission definitions per role |

## Role Permissions

Each role has table-level permissions:

```json
{
  "roles": {
    "user": {
      "tables": {
        "posts": {
          "select": { ... },
          "insert": { ... }
        }
      },
      "inherit_from": "anonymous"
    }
  }
}
```

### Role Options

| Option | Type | Description |
|--------|------|-------------|
| `tables` | object | Table name → permissions mapping |
| `inherit_from` | string | Inherit permissions from another role |

## Select Permissions

Control what data can be queried:

```json
{
  "select": {
    "columns": ["id", "name", "email", "created_at"],
    "filter": {
      "is_deleted": { "_eq": false }
    },
    "limit": 100,
    "allow_aggregations": true
  }
}
```

### Select Options

| Option | Type | Description |
|--------|------|-------------|
| `columns` | array | Columns user can select (empty = all) |
| `filter` | object | Row-level security filter |
| `limit` | integer | Maximum rows returned |
| `allow_aggregations` | boolean | Allow aggregate queries |

## Insert Permissions

Control what data can be inserted:

```json
{
  "insert": {
    "columns": ["title", "content", "category_id"],
    "check": {
      "category_id": { "_in": [1, 2, 3] }
    },
    "set": {
      "author_id": "X-User-Id",
      "created_at": "now()"
    }
  }
}
```

### Insert Options

| Option | Type | Description |
|--------|------|-------------|
| `columns` | array | Columns user can provide |
| `check` | object | Validation filter for new rows |
| `set` | object | Columns automatically set |

## Row-Level Security Filters

Filters restrict which rows a user can access based on their session.

### Basic Filter

```json
{
  "filter": {
    "user_id": { "_eq": "X-User-Id" }
  }
}
```

This ensures users can only access their own data.

### Combining Conditions

**AND conditions:**
```json
{
  "filter": {
    "_and": [
      { "status": { "_eq": "published" } },
      { "is_deleted": { "_eq": false } }
    ]
  }
}
```

**OR conditions:**
```json
{
  "filter": {
    "_or": [
      { "is_public": { "_eq": true } },
      { "author_id": { "_eq": "X-User-Id" } }
    ]
  }
}
```

**NOT condition:**
```json
{
  "filter": {
    "_not": {
      "status": { "_eq": "draft" }
    }
  }
}
```

### Comparison Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `_eq` | Equal | `{ "status": { "_eq": "active" } }` |
| `_neq` | Not equal | `{ "status": { "_neq": "deleted" } }` |
| `_gt` | Greater than | `{ "age": { "_gt": 18 } }` |
| `_gte` | Greater than or equal | `{ "price": { "_gte": 100 } }` |
| `_lt` | Less than | `{ "stock": { "_lt": 10 } }` |
| `_lte` | Less than or equal | `{ "quantity": { "_lte": 5 } }` |
| `_in` | In array | `{ "category": { "_in": ["a", "b"] } }` |
| `_like` | SQL LIKE pattern | `{ "email": { "_like": "%@company.com" } }` |

## Session Variables

Reference user session data in filters:

### Common Session Variables

| Variable | Description |
|----------|-------------|
| `X-User-Id` | Current user's ID |
| `X-Role` | Current role |
| `X-Org-Id` | User's organization ID |

### Using Session Variables

```json
{
  "filter": {
    "tenant_id": { "_eq": "X-Tenant-Id" },
    "created_by": { "_eq": "X-User-Id" }
  }
}
```

## Complete Examples

### Multi-Tenant Application

```json
{
  "permissions": {
    "admin_role": "admin",
    "default_role": "tenant_user",
    "roles": {
      "admin": {
        "tables": {
          "users": { "select": {}, "insert": {} },
          "organizations": { "select": {}, "insert": {} },
          "data": { "select": {}, "insert": {} }
        }
      },
      "org_admin": {
        "tables": {
          "users": {
            "select": {
              "filter": {
                "org_id": { "_eq": "X-Org-Id" }
              }
            },
            "insert": {
              "set": {
                "org_id": "X-Org-Id"
              }
            }
          },
          "data": {
            "select": {
              "filter": {
                "org_id": { "_eq": "X-Org-Id" }
              },
              "allow_aggregations": true
            },
            "insert": {
              "set": {
                "org_id": "X-Org-Id",
                "created_by": "X-User-Id"
              }
            }
          }
        }
      },
      "tenant_user": {
        "tables": {
          "data": {
            "select": {
              "filter": {
                "_and": [
                  { "org_id": { "_eq": "X-Org-Id" } },
                  { "created_by": { "_eq": "X-User-Id" } }
                ]
              }
            }
          }
        }
      }
    }
  }
}
```

### Blog Application

```json
{
  "permissions": {
    "admin_role": "admin",
    "default_role": "anonymous",
    "roles": {
      "admin": {
        "tables": {
          "posts": {
            "select": { "allow_aggregations": true },
            "insert": {}
          },
          "users": {
            "select": { "allow_aggregations": true },
            "insert": {}
          }
        }
      },
      "author": {
        "tables": {
          "posts": {
            "select": {
              "filter": {
                "_or": [
                  { "status": { "_eq": "published" } },
                  { "author_id": { "_eq": "X-User-Id" } }
                ]
              }
            },
            "insert": {
              "columns": ["title", "content", "category_id"],
              "set": {
                "author_id": "X-User-Id",
                "status": "draft"
              }
            }
          },
          "users": {
            "select": {
              "columns": ["id", "name", "avatar_url"]
            }
          }
        }
      },
      "anonymous": {
        "tables": {
          "posts": {
            "select": {
              "filter": {
                "status": { "_eq": "published" }
              },
              "limit": 50
            }
          },
          "users": {
            "select": {
              "columns": ["id", "name"]
            }
          }
        }
      }
    }
  }
}
```

### E-Commerce Application

```json
{
  "permissions": {
    "admin_role": "admin",
    "default_role": "customer",
    "roles": {
      "admin": {
        "tables": {
          "products": { "select": {}, "insert": {} },
          "orders": { "select": {}, "insert": {} },
          "customers": { "select": {} }
        }
      },
      "customer": {
        "tables": {
          "products": {
            "select": {
              "filter": {
                "is_active": { "_eq": true }
              }
            }
          },
          "orders": {
            "select": {
              "filter": {
                "customer_id": { "_eq": "X-User-Id" }
              }
            },
            "insert": {
              "columns": ["product_id", "quantity", "shipping_address"],
              "set": {
                "customer_id": "X-User-Id",
                "status": "pending"
              }
            }
          }
        }
      },
      "anonymous": {
        "tables": {
          "products": {
            "select": {
              "filter": {
                "_and": [
                  { "is_active": { "_eq": true } },
                  { "stock": { "_gt": 0 } }
                ]
              },
              "columns": ["id", "name", "price", "description", "image_url"],
              "limit": 100
            }
          }
        }
      }
    }
  }
}
```

## Testing Permissions

Test your permission configuration:

```bash
# Validate configuration
./bin/ndc-clickhouse validate --config ./config

# Test as specific role
curl -X POST http://localhost:8080/query \
  -H "X-Role: customer" \
  -H "X-User-Id: 123" \
  -d '{ ... }'
```

## Best Practices

1. **Start restrictive**: Begin with minimal permissions and expand as needed
2. **Use role inheritance**: Create base roles and inherit from them
3. **Document roles**: Add descriptions explaining what each role can do
4. **Test thoroughly**: Test each role with different session variables
5. **Audit regularly**: Review permissions periodically
6. **Use session variables**: Never hardcode user IDs in filters
