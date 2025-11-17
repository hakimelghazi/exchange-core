-- name: CreateUser :one
INSERT INTO users (
    id, email
) VALUES (
    $1, $2
)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: UpsertUser :exec
INSERT INTO users (id, email)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING;
