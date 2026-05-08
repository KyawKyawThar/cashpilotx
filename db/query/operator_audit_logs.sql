-- ═══════════════════════════════════════════════════════
-- CREATE
-- ═══════════════════════════════════════════════════════

-- name: CreateOperatorAuditLog :one
INSERT INTO operator_audit_log (
    user_id,
    performed_by,
    action_type,
    entity_type,
    action_label,
    detail,
    invoice_id,
    invoice_number,
    customer_id,
    payment_id,
    reminder_id,
    message_id,
    metadata,
    request_id
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12,
    $13,
    $14
)
RETURNING *;



-- ═══════════════════════════════════════════════════════
-- READ SINGLE
-- ═══════════════════════════════════════════════════════

-- name: GetOperatorAuditLog :one
SELECT *
FROM operator_audit_log
WHERE id = $1
LIMIT 1;



-- ═══════════════════════════════════════════════════════
-- LIST
-- ═══════════════════════════════════════════════════════

-- name: ListOperatorAuditLogs :many
SELECT *
FROM operator_audit_log
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2
OFFSET $3;



-- name: ListOperatorAuditLogsCursor :many
SELECT *
FROM operator_audit_log
WHERE user_id = $1
  AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;



-- name: ListOperatorAuditLogsByInvoice :many
SELECT *
FROM operator_audit_log
WHERE invoice_id = $1
ORDER BY created_at DESC;



-- name: ListOperatorAuditLogsByCustomer :many
SELECT *
FROM operator_audit_log
WHERE customer_id = $1
ORDER BY created_at DESC;



-- name: ListOperatorAuditLogsByActionType :many
SELECT *
FROM operator_audit_log
WHERE user_id = $1
  AND action_type = $2
ORDER BY created_at DESC
LIMIT $3
OFFSET $4;



-- name: ListRevertedOperatorAuditLogs :many
SELECT *
FROM operator_audit_log
WHERE user_id = $1
  AND is_reverted = TRUE
ORDER BY created_at DESC
LIMIT $2
OFFSET $3;



-- ═══════════════════════════════════════════════════════
-- SEARCH
-- ═══════════════════════════════════════════════════════

-- name: SearchOperatorAuditLogs :many
SELECT *
FROM operator_audit_log
WHERE user_id = sqlc.arg(user_id)
  AND (
      action_label ILIKE '%' || sqlc.arg(keyword) || '%'
      OR detail ILIKE '%' || sqlc.arg(keyword) || '%'
      OR invoice_number ILIKE '%' || sqlc.arg(keyword) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);



-- ═══════════════════════════════════════════════════════
-- REVERT
-- ═══════════════════════════════════════════════════════

-- name: MarkOperatorAuditLogReverted :one
UPDATE operator_audit_log
SET
    is_reverted = TRUE,
    reverted_by = $2,
    reverted_at = now()
WHERE id = $1
RETURNING *;



-- ═══════════════════════════════════════════════════════
-- ANALYTICS
-- ═══════════════════════════════════════════════════════

-- name: CountOperatorAuditLogs :one
SELECT COUNT(*)::BIGINT
FROM operator_audit_log
WHERE user_id = $1;



-- name: CountOperatorAuditLogsByActionType :one
SELECT COUNT(*)::BIGINT
FROM operator_audit_log
WHERE user_id = $1
  AND action_type = $2;



-- name: CountOperatorAuditLogsByInvoice :one
SELECT COUNT(*)::BIGINT
FROM operator_audit_log
WHERE invoice_id = $1;



-- name: GetOperatorAuditSummary :one
SELECT
    COUNT(*)::BIGINT AS total_actions,

    COUNT(*) FILTER (
        WHERE action_type = 'payment_recorded'
    )::BIGINT AS payments_recorded,

    COUNT(*) FILTER (
        WHERE action_type = 'message_marked_sent'
    )::BIGINT AS messages_sent,

    COUNT(*) FILTER (
        WHERE action_type = 'collection_state_changed'
    )::BIGINT AS state_changes,

    COUNT(*) FILTER (
        WHERE is_reverted = TRUE
    )::BIGINT AS reverted_actions

FROM operator_audit_log
WHERE user_id = $1;



-- ═══════════════════════════════════════════════════════
-- IDEMPOTENCY
-- ═══════════════════════════════════════════════════════

-- name: GetOperatorAuditLogByRequestID :one
SELECT *
FROM operator_audit_log
WHERE user_id = $1
  AND request_id = $2
LIMIT 1;
