-- "get all open orders for market"
CREATE INDEX IF NOT EXISTS idx_orders_market_status
  ON orders (market, status);

-- "all trades for an order"
CREATE INDEX IF NOT EXISTS idx_trades_taker_order
  ON trades (taker_order_id);

CREATE INDEX IF NOT EXISTS idx_trades_maker_order
  ON trades (maker_order_id);

-- "show ledger entries for account"
CREATE INDEX IF NOT EXISTS idx_ledger_entries_account
  ON ledger_entries (account_id);
