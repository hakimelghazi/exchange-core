-- users: just to own accounts
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT UNIQUE
);

-- accounts: per-user per-asset balance
CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    asset TEXT NOT NULL,                   -- 'USD', 'BTC'
    balance NUMERIC(20, 8) NOT NULL DEFAULT 0
);

-- orders: what the engine knows about
CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    market TEXT NOT NULL,                  -- 'BTC-USD'
    side TEXT NOT NULL,                    -- 'BUY' | 'SELL'
    price NUMERIC(20, 8),                  -- null for pure market
    quantity NUMERIC(20, 8) NOT NULL,
    remaining NUMERIC(20, 8) NOT NULL,
    status TEXT NOT NULL,                  -- 'OPEN','PARTIAL','FILLED','CANCELLED'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- trades emitted by the engine
CREATE TABLE trades (
    id UUID PRIMARY KEY,
    taker_order_id UUID NOT NULL REFERENCES orders(id),
    maker_order_id UUID NOT NULL REFERENCES orders(id),
    price NUMERIC(20, 8) NOT NULL,
    quantity NUMERIC(20, 8) NOT NULL,
    traded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
