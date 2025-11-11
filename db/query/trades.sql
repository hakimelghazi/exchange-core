-- name: InsertTrade :one
INSERT INTO trades (
    id, taker_order_id,maker_order_id, price, quantity
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: ListTradesByOrder :many
SELECT * FROM trades
WHERE taker_order_id = $1 OR maker_order_id = $1
ORDER BY traded_at DESC;