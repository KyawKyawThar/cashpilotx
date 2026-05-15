-- name: CreatePayment :one
INSERT INTO payments (
    invoice_id,
    user_id,
    amount,
    currency,
    method,
    reference,
    proof_url,
    note,
    paid_on
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING *;



-- name: GetPaymentByID :one
SELECT *
FROM payments
WHERE id = $1
  AND user_id = $2
LIMIT 1;


-- name: ListPaymentsByInvoice :many
SELECT *
FROM payments
WHERE user_id = $1
  AND invoice_id = $2
ORDER BY created_at DESC;


-- name: ListPayments :many
SELECT *
FROM payments
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;


-- name: ListPaymentsByCustomer :many
SELECT p.*
FROM payments p
JOIN invoices i ON p.invoice_id = i.id
WHERE p.user_id = $1
  AND i.customer_id = $2
ORDER BY p.created_at DESC
LIMIT $3 OFFSET $4;


-- name: SumPaymentsForInvoice :one
SELECT COALESCE(SUM(amount), 0)::BIGINT
FROM payments
WHERE invoice_id = $1;


-- name: SumCollectedThisMonth :one
-- Real-time sum of payments recorded in the current calendar month.
SELECT COALESCE(SUM(amount), 0)::BIGINT AS total_collected
FROM payments
WHERE user_id = $1
  AND date_trunc('month', paid_on) = date_trunc('month', now());


-- name: SumCollectedBetween :one
-- Collected amount and payment count for a custom date range (to is exclusive).
SELECT
  COALESCE(SUM(amount), 0)::BIGINT AS collected,
  COUNT(*)::INT                     AS payment_count
FROM payments
WHERE user_id = $1
  AND paid_on >= $2
  AND paid_on <  $3;


-- name: SetPaymentVoided :one
UPDATE payments
SET voided_at = now()
WHERE id      = $1
  AND user_id = $2
  AND voided_at IS NULL
RETURNING *;


-- name: ListPaymentsEnriched :many
SELECT
  p.*,
  c.name   AS customer_name,
  i.external_id AS invoice_number,
  i.status  AS invoice_status
FROM payments p
JOIN invoices  i ON i.id = p.invoice_id
JOIN customers c ON c.id = i.customer_id
WHERE p.user_id = $1
ORDER BY p.created_at DESC
LIMIT $2 OFFSET $3;
