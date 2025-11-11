-- name: CreateOrder :one
INSERT INTO orders (
    id, user_id, market, side, price, quantity, remaining, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;
