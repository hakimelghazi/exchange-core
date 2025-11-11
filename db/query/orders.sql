-- name: CreateOrder :one
INSERT INTO orders (
    id, user_id, market, side, price, quantity, remaining, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: UpdateOrderStatus :exec
UPDATE orders
SET status = $2, remaining = $3
WHERE id = $1;