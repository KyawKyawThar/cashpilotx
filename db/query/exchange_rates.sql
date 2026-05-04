-- name: CloseCurrentExchangeRate :exec
UPDATE exchange_rates
SET valid_to   = $4,
    updated_at = now()
WHERE user_id       = $1
  AND from_currency = $2
  AND to_currency   = $3
  AND valid_to IS NULL;


-- name: CreateExchangeRate :one
INSERT INTO exchange_rates (
    user_id,
    from_currency,
    to_currency,
    rate,
    source,
    valid_from
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;


-- name: GetActiveExchangeRate :one
SELECT *
FROM exchange_rates
WHERE user_id = $1
  AND from_currency = $2
  AND to_currency = $3
  AND valid_to IS NULL
LIMIT 1;


-- name: GetExchangeRateAtTime :one
SELECT *
FROM exchange_rates
WHERE user_id = $1
  AND from_currency = $2
  AND to_currency = $3
  AND valid_from <= $4
  AND (valid_to IS NULL OR valid_to > $4)
ORDER BY valid_from DESC
LIMIT 1;


-- name: ListExchangeRates :many
SELECT *
FROM exchange_rates
WHERE user_id = $1
  AND from_currency = $2
  AND to_currency = $3
ORDER BY valid_from DESC
LIMIT $4 OFFSET $5;


-- name: CloseExchangeRateByID :exec
UPDATE exchange_rates
SET valid_to   = now(),
    updated_at = now()
WHERE id = $1
  AND valid_to IS NULL;