-- name: CreateCustomerEvent :one
INSERT INTO customer_events (
    user_id,
    customer_id,
    invoice_id,
    event_type,
    event_value
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetInvoiceTimeline :many
-- Powers GET /api/v1/invoices/{id}/timeline — History modal in v3.
-- Returns all events for a specific invoice, newest first.
SELECT *
FROM customer_events
WHERE invoice_id = $1
ORDER BY created_at DESC;

-- name: ListCustomerEvents :many
SELECT *
FROM customer_events
WHERE user_id = $1
  AND customer_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListEventsByType :many
SELECT *
FROM customer_events
WHERE user_id = $1
  AND event_type = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: CountEventsByType :many
SELECT event_type, COUNT(*) AS total
FROM customer_events
WHERE user_id = $1
  AND created_at BETWEEN $2 AND $3
GROUP BY event_type;
