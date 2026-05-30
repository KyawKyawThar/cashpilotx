-- name: CreateCustomer :one
INSERT INTO customers (
    user_id,
    name,
    normalized_name,
    phone,
    normalized_phone,
    country,
    type,
    notes,
    tags,
    customer_number,
    assigned_to
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *;



-- name: GetCustomerByID :one
SELECT *
FROM customers
WHERE id = $1
  AND user_id = $2
LIMIT 1;

-- name: GetCustomerByPhone :one
SELECT *
FROM customers
WHERE user_id = $1
  AND normalized_phone = $2
LIMIT 1;


-- name: ListCustomers :many
SELECT *
FROM customers
WHERE user_id = $1
  AND status = 'active'
ORDER BY last_activity DESC NULLS LAST, created_at DESC
LIMIT $2 OFFSET $3;


-- name: ListAllCustomers :many
SELECT *
FROM customers
WHERE user_id = $1
ORDER BY status ASC, last_activity DESC NULLS LAST, created_at DESC
LIMIT $2 OFFSET $3;


-- name: SearchCustomers :many
SELECT *
FROM customers
WHERE user_id = $1
  AND status = 'active'
  AND (
    normalized_name ILIKE '%' || $2 || '%'
    OR normalized_phone ILIKE '%' || $2 || '%'
    OR customer_number ILIKE '%' || $2 || '%'
  )
ORDER BY last_activity DESC
LIMIT 20;


-- name: UpdateCustomer :one
UPDATE customers
SET
    name = COALESCE(sqlc.narg(name), name),
    normalized_name = COALESCE(sqlc.narg(normalized_name), normalized_name),
    phone = COALESCE(sqlc.narg(phone), phone),
    normalized_phone = COALESCE(sqlc.narg(normalized_phone), normalized_phone),
    country = COALESCE(sqlc.narg(country), country),
    notes = COALESCE(sqlc.narg(notes), notes),
    tags = COALESCE(sqlc.narg(tags), tags),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;


-- name: AddCustomerDebt :one
UPDATE customers
SET
    total_owed = total_owed + $3,
    last_activity = now(),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- name: AddCustomerPayment :one
UPDATE customers
SET
    total_paid = total_paid + $3,
    total_owed = GREATEST(0, total_owed - $3),
    last_activity = now(),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;


-- name: SubtractCustomerPayment :one
UPDATE customers
SET
    total_paid = GREATEST(0, total_paid - $3),
    total_owed = total_owed + $3,
    last_activity = now(),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;


-- name: SetCustomerStatus :exec
UPDATE customers
SET
    status = $3,
    updated_at = now()
WHERE id = $1
  AND user_id = $2;


-- name: ListCustomersAssigned :many
SELECT *
FROM customers
WHERE user_id = $1
  AND assigned_to = $2
  AND status = 'active'
ORDER BY last_activity DESC NULLS LAST, created_at DESC
LIMIT $3 OFFSET $4;

-- name: TopDebtors :many
SELECT *
FROM customers
WHERE user_id = $1
  AND total_owed > 0
ORDER BY total_owed DESC
LIMIT $2;

-- name: GetCustomersByIDs :many
SELECT id, name, phone, customer_number
FROM customers
WHERE user_id = $1
  AND id = ANY($2::uuid[])
ORDER BY name;