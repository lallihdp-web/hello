-- Sample tables for testing the ClickHouse connector

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID DEFAULT generateUUIDv4(),
    name String,
    email String,
    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now(),
    is_active Bool DEFAULT true,
    age Nullable(UInt8),
    metadata JSON
) ENGINE = MergeTree()
ORDER BY (created_at, id);

-- Products table
CREATE TABLE IF NOT EXISTS products (
    id UUID DEFAULT generateUUIDv4(),
    name String,
    description Nullable(String),
    price Decimal(10, 2),
    category LowCardinality(String),
    tags Array(String),
    stock UInt32 DEFAULT 0,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (category, created_at, id);

-- Orders table
CREATE TABLE IF NOT EXISTS orders (
    id UUID DEFAULT generateUUIDv4(),
    user_id UUID,
    product_id UUID,
    quantity UInt32,
    total_price Decimal(10, 2),
    status Enum8('pending' = 1, 'processing' = 2, 'shipped' = 3, 'delivered' = 4, 'cancelled' = 5),
    order_date DateTime DEFAULT now(),
    shipped_at Nullable(DateTime)
) ENGINE = MergeTree()
ORDER BY (order_date, id);

-- Events table (for analytics)
CREATE TABLE IF NOT EXISTS events (
    id UUID DEFAULT generateUUIDv4(),
    event_type LowCardinality(String),
    user_id Nullable(UUID),
    timestamp DateTime64(3) DEFAULT now64(3),
    properties JSON,
    value Float64 DEFAULT 0
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (event_type, timestamp, id);

-- Insert sample data
INSERT INTO users (name, email, age) VALUES
    ('Alice Johnson', 'alice@example.com', 28),
    ('Bob Smith', 'bob@example.com', 35),
    ('Carol White', 'carol@example.com', 42);

INSERT INTO products (name, description, price, category, tags, stock) VALUES
    ('Laptop Pro', 'High-performance laptop', 1299.99, 'Electronics', ['computer', 'portable'], 50),
    ('Wireless Mouse', 'Ergonomic wireless mouse', 49.99, 'Electronics', ['peripheral', 'wireless'], 200),
    ('Coffee Maker', 'Automatic coffee maker', 89.99, 'Home', ['kitchen', 'appliance'], 75);
