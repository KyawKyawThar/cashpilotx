-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (
    key,
    user_id,
    response
) VALUES (
    $1, $2, $3
)
RETURNING *;


-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys
WHERE key = $1
  AND user_id = $2;


-- name: GetIdempotencyKeyForUpdate :one
SELECT * FROM idempotency_keys
WHERE key = $1
  AND user_id = $2
FOR UPDATE;


-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys
WHERE key = $1
  AND user_id = $2;


-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_keys
WHERE created_at < $1;
