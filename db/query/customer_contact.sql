-- name: UpsertCustomerContact :one
INSERT INTO customer_contacts (
    user_id,
    customer_id,
    platform,
    external_id,
    display_name,
    meta,
    is_active
) VALUES ($1, $2, $3, $4, $5, $6, true)
ON CONFLICT (user_id, platform, external_id)
WHERE is_active = true
DO UPDATE SET
    customer_id = EXCLUDED.customer_id,
    display_name = EXCLUDED.display_name,
    meta = EXCLUDED.meta,
    is_active = true
RETURNING *;


-- name: GetContactByPlatform :one
SELECT * FROM customer_contacts
WHERE user_id = $1
  AND platform = $2
  AND external_id = $3
  AND is_active = true
LIMIT 1;


-- name: ListContactsByPlatform :many
SELECT * FROM customer_contacts
WHERE user_id = $1
  AND platform = $2
  AND is_active = true;


-- name: ListContactsByCustomer :many
SELECT * FROM customer_contacts
WHERE customer_id = $1
  AND is_active = true
ORDER BY created_at DESC, id DESC;


-- name: DeactivateContact :exec
UPDATE customer_contacts
SET is_active = false
WHERE id = $1;


-- name: ReassignContact :one
UPDATE customer_contacts
SET customer_id = $2
WHERE id = $1
RETURNING *;


-- name: ListUserContacts :many
SELECT * FROM customer_contacts
WHERE user_id = $1
  AND is_active = true
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;


-- name: DeleteContact :exec
DELETE FROM customer_contacts
WHERE id = $1;