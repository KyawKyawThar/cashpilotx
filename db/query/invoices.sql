-- name: CreateInvoice :one
INSERT INTO invoices (
    user_id,
    customer_id,
    amount,
    currency,
    description,
    due_date,
    source,
    photo_url,
    external_id,
    items,
    assigned_to,
    delivery_service,
    tracking_number
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, COALESCE($10::jsonb, '[]'::jsonb), $11, $12, $13)
RETURNING *;

-- name: GetInvoiceByID :one
SELECT *
FROM invoices
WHERE id = $1
  AND user_id = $2
LIMIT 1;

-- name: ListInvoices :many
SELECT *
FROM invoices
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListInvoicesByStatus :many
SELECT *
FROM invoices
WHERE user_id = $1
  AND status = $2
ORDER BY due_date ASC
LIMIT $3 OFFSET $4;

-- name: ListInvoicesByCustomer :many
SELECT *
FROM invoices
WHERE user_id = $1
  AND customer_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListOverdueInvoices :many
SELECT *
FROM invoices
WHERE user_id = $1
  AND paid_amount < amount
  AND due_date < CURRENT_DATE
ORDER BY due_date ASC
LIMIT $2 OFFSET $3;

-- name: AddPayment :one
UPDATE invoices
SET
    paid_amount = paid_amount + $3,
    updated_at  = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- name: RecomputeInvoiceStatus :one
WITH paid AS (
    SELECT COALESCE(SUM(amount), 0)::BIGINT AS total
    FROM payments
    WHERE invoice_id = $1
      AND voided_at IS NULL
)
UPDATE invoices
SET
    paid_amount = paid.total,
    status = CASE
        WHEN paid.total = 0           THEN 'unpaid'
        WHEN paid.total < invoices.amount THEN 'partial'
        ELSE                               'paid'
    END,
    paid_at = CASE
        WHEN paid.total >= invoices.amount AND paid_at IS NULL THEN now()
        ELSE paid_at
    END,
    updated_at = now()
FROM paid
WHERE invoices.id = $1
  AND invoices.user_id = $2
RETURNING invoices.*;

-- name: CancelInvoice :one
UPDATE invoices
SET
    status     = 'cancelled',
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND status != 'cancelled'
RETURNING *;

-- ─── Collection lifecycle ─────────────────────────────────────────────────────

-- name: UpdateInvoiceCollectionState :one
-- General state move: used by MoveCollectionStateTx, RecordPaymentTx (paid),
-- WriteOffInvoiceTx (defaulted), CreateInvoiceTx (pending init).
UPDATE invoices
SET
    collection_state = $3,
    updated_at       = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- name: SetInvoiceContacted :one
-- Called by FireReminderTx after a reminder is sent.
-- Advances collection_state, stamps last_contacted_at, increments follow_up_count.
UPDATE invoices
SET
    collection_state  = $3,
    last_contacted_at = now(),
    follow_up_count   = follow_up_count + 1,
    updated_at        = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- name: SetInvoiceReplied :one
-- Called by ProcessInboundReplyTx after an inbound reply is classified.
-- Advances collection_state and stamps last_response_at.
UPDATE invoices
SET
    collection_state = $3,
    last_response_at = now(),
    updated_at       = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- ─── Recovery Queue (confidence scoring) ─────────────────────────────────────

-- name: UpdateInvoiceConfidence :execrows
-- Written by RefreshRecoveryQueueTx (background job, runs after each event).
-- confidence_factors shape: [{label, signal, color: "green"|"amber"|"red"}]
UPDATE invoices
SET
    confidence_score       = $3,
    confidence_factors     = $4,
    outcome_text           = $5,
    outcome_timeframe      = $6,
    confidence_computed_at = now(),
    updated_at             = now()
WHERE id = $1
  AND user_id = $2;

-- name: GetRecoveryQueue :many
-- Powers GET /api/v1/recovery-queue — open invoices ranked by AI confidence score.
-- Returns invoice + joined customer name/phone so the handler needs no second query.
SELECT
    i.*,
    c.name  AS customer_name,
    c.phone AS customer_phone
FROM invoices i
JOIN customers c ON i.customer_id = c.id
WHERE i.user_id = $1
  AND i.status NOT IN ('paid', 'cancelled')
ORDER BY
    i.confidence_score DESC NULLS LAST,
    i.due_date ASC
LIMIT $2 OFFSET $3;

-- name: ListUnscored :many
-- Used by RefreshRecoveryQueueTx to find invoices that have never been scored
-- or whose score is stale (not computed in the last 30 minutes).
SELECT id, user_id, customer_id
FROM invoices
WHERE status NOT IN ('paid', 'cancelled')
  AND (
        confidence_computed_at IS NULL
        OR confidence_computed_at < now() - INTERVAL '30 minutes'
      )
ORDER BY due_date ASC
LIMIT $1;

-- ─── Dashboard stats strip ────────────────────────────────────────────────────

-- name: SetInvoicePromised :one
-- Called by ProcessInboundReplyTx when the customer commits to a payment date.
-- Advances collection_state to 'promised', records promised_at and last_response_at.
UPDATE invoices
SET
    collection_state = 'promised',
    promised_at      = $3,
    last_response_at = now(),
    updated_at       = now()
WHERE id = $1
  AND user_id = $2
RETURNING *;

-- name: GetRecoveryStats :one
-- Powers the v3 stats strip (Recoverable now / Waiting for reply / Open actions).
-- "Recovered this month" comes from GetMonthlyStats in daily_stats.sql.
SELECT
    COALESCE(SUM(amount - paid_amount)
        FILTER (WHERE status NOT IN ('paid','cancelled')), 0)::BIGINT  AS total_recoverable,
    COALESCE(SUM(amount - paid_amount)
        FILTER (WHERE collection_state = 'reminded'), 0)::BIGINT       AS waiting_for_reply,
    COUNT(*)
        FILTER (WHERE status NOT IN ('paid','cancelled')
            AND collection_state NOT IN ('reminded','no_response','paid','defaulted'))::INT AS open_actions,
    COUNT(*)
        FILTER (WHERE collection_state = 'reminded')::INT              AS reminded_count,
    COUNT(*)
        FILTER (WHERE status NOT IN ('paid','cancelled'))::INT         AS open_invoices_count,
    COALESCE(SUM(amount)      FILTER (WHERE status != 'cancelled'), 0)::BIGINT AS total_invoiced,
    COALESCE(SUM(paid_amount) FILTER (WHERE status != 'cancelled'), 0)::BIGINT AS total_collected
FROM invoices
WHERE user_id = $1;

-- name: ListInvoicesDueToday :many
SELECT id, user_id, customer_id, amount, currency, paid_amount, paid_at, description, due_date, status, source, photo_url, external_id, issued_at, items, collection_state, last_contacted_at, last_response_at, follow_up_count, confidence_score, confidence_factors, outcome_text, outcome_timeframe, confidence_computed_at, created_at, updated_at, promised_at
FROM invoices
WHERE user_id = $1
  AND paid_amount < amount
  AND due_date = CURRENT_DATE
  AND status NOT IN ('cancelled', 'paid', 'written_off')
ORDER BY due_date ASC;
