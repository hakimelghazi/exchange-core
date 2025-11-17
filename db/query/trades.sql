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

-- name: ListTrades :many
SELECT t.*
FROM trades t
JOIN orders ot ON ot.id = t.taker_order_id
JOIN orders om ON om.id = t.maker_order_id
WHERE ($1::uuid IS NULL OR ot.user_id = $1 OR om.user_id = $1)
  AND ($2::uuid IS NULL OR t.taker_order_id = $2 OR t.maker_order_id = $2)
  AND ($3::text = '' OR ot.market = $3 OR om.market = $3)
  AND ($4::timestamptz IS NULL OR t.traded_at >= $4)
ORDER BY t.traded_at DESC
LIMIT $5;
