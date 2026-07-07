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
-- $2 is the lowercased raw query (name / customer_number). $3 is the
-- digits-only, leading-zero-stripped phone query (see normalizePhoneQuery in
-- customer.service.go) — the raw query can't ILIKE-match normalized_phone
-- directly since stored numbers have no spaces/dashes and use +95 while
-- users often type the local 0-prefixed form.
SELECT *
FROM customers
WHERE user_id = $1
  AND status = 'active'
  AND (
    normalized_name ILIKE '%' || $2 || '%'
    OR ($3 <> '' AND normalized_phone ILIKE '%' || $3 || '%')
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

-- name: AssignCustomer :one
-- Reassigns customer ownership to another team member (or clears it when
-- assigned_to is null). Distinct from UpdateCustomer since it's a
-- higher-privilege action gated by the customer:edit permission.
UPDATE customers
SET
    assigned_to = $3,
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


-- name: CountCustomers :one
SELECT COUNT(*)
FROM customers
WHERE user_id = $1
  AND status = 'active';

-- name: CountCustomersAssigned :one
SELECT COUNT(*)
FROM customers
WHERE user_id = $1
  AND assigned_to = $2
  AND status = 'active';

-- name: ListCustomersHasOpenInvoice :many
SELECT c.*
FROM customers c
WHERE c.user_id = $1
  AND c.status = 'active'
  AND ($2::uuid IS NULL OR c.assigned_to = $2)
  AND EXISTS (
    SELECT 1 FROM invoices i
    WHERE i.customer_id = c.id
      AND i.user_id = c.user_id
      AND i.status NOT IN ('paid', 'cancelled', 'returned')
  )
ORDER BY c.last_activity DESC NULLS LAST, c.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountCustomersHasOpenInvoice :one
SELECT COUNT(*)
FROM customers c
WHERE c.user_id = $1
  AND c.status = 'active'
  AND ($2::uuid IS NULL OR c.assigned_to = $2)
  AND EXISTS (
    SELECT 1 FROM invoices i
    WHERE i.customer_id = c.id
      AND i.user_id = c.user_id
      AND i.status NOT IN ('paid', 'cancelled', 'returned')
  );

-- name: ListCustomersNoOpenInvoice :many
SELECT c.*
FROM customers c
WHERE c.user_id = $1
  AND c.status = 'active'
  AND ($2::uuid IS NULL OR c.assigned_to = $2)
  AND NOT EXISTS (
    SELECT 1 FROM invoices i
    WHERE i.customer_id = c.id
      AND i.user_id = c.user_id
      AND i.status NOT IN ('paid', 'cancelled', 'returned')
  )
ORDER BY c.last_activity DESC NULLS LAST, c.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountCustomersNoOpenInvoice :one
SELECT COUNT(*)
FROM customers c
WHERE c.user_id = $1
  AND c.status = 'active'
  AND ($2::uuid IS NULL OR c.assigned_to = $2)
  AND NOT EXISTS (
    SELECT 1 FROM invoices i
    WHERE i.customer_id = c.id
      AND i.user_id = c.user_id
      AND i.status NOT IN ('paid', 'cancelled', 'returned')
  );

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

-- name: ListDuplicateCustomerCandidatesNoInvoice :many
-- Flags customers with no open invoice whose phone number is shared by
-- another customer record in the same org — the signature of the same real
-- person being entered twice (e.g. by two sales reps who can't see each
-- other's customers). Surfaced on the "No Invoice" tab so these can be
-- reviewed/deactivated before any order work happens.
SELECT c.*
FROM customers c
WHERE c.user_id = $1
  AND c.status = 'active'
  AND c.normalized_phone IS NOT NULL
  AND c.normalized_phone <> ''
  AND NOT EXISTS (
    SELECT 1 FROM invoices i
    WHERE i.customer_id = c.id
      AND i.user_id = c.user_id
      AND i.status NOT IN ('paid', 'cancelled', 'returned')
  )
  AND EXISTS (
    SELECT 1 FROM customers c2
    WHERE c2.user_id = c.user_id
      AND c2.normalized_phone = c.normalized_phone
      AND c2.id <> c.id
  )
ORDER BY c.normalized_phone, c.created_at DESC;