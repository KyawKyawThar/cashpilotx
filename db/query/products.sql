-- name: CreateProduct :one
INSERT INTO products (org_owner_id, name, sku, origin, category, unit, cost_price, selling_price, currency, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetProduct :one
SELECT * FROM products
WHERE id = $1 AND org_owner_id = $2;

-- name: ListProducts :many
SELECT * FROM products
WHERE org_owner_id = $1
  AND is_active = true
ORDER BY name ASC;

-- name: ListAllProducts :many
SELECT * FROM products
WHERE org_owner_id = $1
ORDER BY name ASC;

-- name: UpdateProduct :one
UPDATE products
SET
    name          = COALESCE(sqlc.narg(name), name),
    sku           = COALESCE(sqlc.narg(sku), sku),
    origin        = COALESCE(sqlc.narg(origin), origin),
    category      = COALESCE(sqlc.narg(category), category),
    unit          = COALESCE(sqlc.narg(unit), unit),
    cost_price    = COALESCE(sqlc.narg(cost_price), cost_price),
    selling_price = COALESCE(sqlc.narg(selling_price), selling_price),
    currency      = COALESCE(sqlc.narg(currency), currency),
    updated_at    = now()
WHERE id = $1 AND org_owner_id = $2
RETURNING *;

-- name: DeactivateProduct :exec
UPDATE products
SET is_active = false, updated_at = now()
WHERE id = $1 AND org_owner_id = $2;
