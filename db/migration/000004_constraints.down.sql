ALTER TABLE ledger_entries
  DROP CONSTRAINT IF EXISTS ledger_entries_amount_not_zero;

ALTER TABLE orders
  DROP CONSTRAINT IF EXISTS orders_status_chk;

ALTER TABLE orders
  DROP CONSTRAINT IF EXISTS orders_side_chk;
