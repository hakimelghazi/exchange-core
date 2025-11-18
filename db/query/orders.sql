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

-- name: MarkOrderCancelled :exec
UPDATE orders
SET status = 'CANCELLED'
WHERE id = $1
  AND status IN ('OPEN','PARTIAL');

-- name: ListOrders :many
SELECT *
FROM orders
WHERE (
        $1::uuid IS NULL
        OR user_id = $1
      )
  AND (
        COALESCE($2, '') = ''
        OR status = $2
      )
  AND (
        COALESCE($3, '') = ''
        OR side = $3
      )
  AND (
        $4::timestamptz IS NULL
        OR (created_at, id) > (
              $4::timestamptz,
              COALESCE($5::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
          )
      )
ORDER BY created_at, id
LIMIT $6;
-- Keyset pagination with (created_at, id)


-- name: ListRestingAsks :many
SELECT *
FROM orders
WHERE status IN ('OPEN','PARTIAL')
  AND side = 'SELL'
  AND (
        $1::text IS NULL
        OR $1 = ''
        OR market = $1
      )
ORDER BY price ASC, created_at ASC, id ASC;

-- name: ListRestingBids :many
SELECT *
FROM orders
WHERE status IN ('OPEN','PARTIAL')
  AND side = 'BUY'
  AND (
        $1::text IS NULL
        OR $1 = ''
        OR market = $1
      )
ORDER BY price DESC, created_at ASC, id ASC;
