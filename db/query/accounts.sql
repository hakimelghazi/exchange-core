-- name: CreateAccount :one
INSERT INTO accounts (
    id, user_id, asset, balance
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: ListAccountsByUser :many
SELECT * FROM accounts WHERE user_id = $1 ORDER BY asset;

-- name: UpdateAccountBalance :one
UPDATE accounts
SET balance = $2
WHERE id = $1
RETURNING *;

-- name: GetAccountByUserAsset :one
SELECT * FROM accounts
WHERE user_id = $1 AND asset = $2;

-- name: UpsertAccount :one
INSERT INTO accounts (id, user_id, asset, balance)  -- balance can remain 0; ledger is source of truth
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, asset) DO NOTHING
RETURNING *;

-- name: GetBalancesByUser :many
SELECT a.asset,
       COALESCE(SUM(le.amount), 0)::NUMERIC(20,8) AS balance
FROM accounts a
LEFT JOIN ledger_entries le ON le.account_id = a.id
WHERE a.user_id = $1
GROUP BY a.asset
ORDER BY a.asset;
