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
