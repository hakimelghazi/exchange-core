CREATE INDEX IF NOT EXISTS idx_orders_open_by_market
  ON orders (market)
  WHERE status IN ('OPEN', 'PARTIAL');