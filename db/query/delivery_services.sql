-- name: ListDeliveryServices :many
SELECT * FROM delivery_services
WHERE user_id = $1
  AND is_active = true
ORDER BY name ASC;

-- name: ListAllDeliveryServices :many
SELECT * FROM delivery_services
WHERE user_id = $1
ORDER BY name ASC;

-- name: CreateDeliveryService :one
INSERT INTO delivery_services (user_id, name, price, currency, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateDeliveryService :one
UPDATE delivery_services
SET
    name     = COALESCE(sqlc.narg(name), name),
    price    = COALESCE(sqlc.narg(price), price),
    currency = COALESCE(sqlc.narg(currency), currency)
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeactivateDeliveryService :exec
UPDATE delivery_services
SET is_active = false
WHERE id = $1 AND user_id = $2;

-- name: DeleteDeliveryService :exec
DELETE FROM delivery_services
WHERE id = $1 AND user_id = $2;
