-- name: UpsertOrder :one
INSERT INTO orders (
    id, user_id, market, side, price, quantity, remaining, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (id) DO UPDATE
SET remaining = EXCLUDED.remaining,
    status    = EXCLUDED.status
RETURNING *;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: GetOrderForUpdate :one
SELECT * FROM orders
WHERE id = $1
FOR UPDATE;

-- name: UpdateOrderAfterMatch :exec
UPDATE orders
SET remaining = $2,
    status = $3
WHERE id = $1;
