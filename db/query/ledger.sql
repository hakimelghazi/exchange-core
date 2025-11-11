-- name: CreateLedger :one
INSERT INTO ledgers (id, ref_type, ref_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: InsertLedgerEntry :exec
INSERT INTO ledger_entries (id, ledger_id, account_id, amount)
VALUES ($1, $2, $3, $4);
