-- name: ListDeliveryServices :many
SELECT * FROM delivery_services
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: CreateDeliveryService :one
INSERT INTO delivery_services (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: DeleteDeliveryService :exec
DELETE FROM delivery_services
WHERE id = $1 AND user_id = $2;
