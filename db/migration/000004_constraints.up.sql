-- limit sides to BUY/SELL
ALTER TABLE orders
  ADD CONSTRAINT orders_side_chk
  CHECK (side IN ('BUY', 'SELL'));

-- limit status to known ones
ALTER TABLE orders
  ADD CONSTRAINT orders_status_chk
  CHECK (status IN ('OPEN', 'PARTIAL', 'FILLED', 'CANCELLED'));

-- make sure amounts in ledger are not zero
ALTER TABLE ledger_entries
  ADD CONSTRAINT ledger_entries_amount_not_zero
  CHECK (amount <> 0);
