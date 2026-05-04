-- name: CreateMessage :one
INSERT INTO messages (
    user_id,
    invoice_id,
    customer_id,
    direction,
    channel,
    content,
    tone,
    status,
    source,
    meta
) VALUES (
    $1, $2, $3,
    $4, $5, $6, $7,
    COALESCE(NULLIF($8::text, '')::message_status, 'draft'),
    COALESCE(NULLIF($9::text, '')::message_source, 'ai'),
    COALESCE($10::jsonb, '{}')
)
RETURNING *;

-- name: ApproveMessage :one
UPDATE messages
SET status = 'pending'
WHERE id = $1
  AND status = 'draft'
RETURNING *;


-- name: MarkMessageSent :one
UPDATE messages
SET
    status = 'sent',
    sent_at = now(),
    external_message_id = $2
WHERE id = $1
RETURNING *;


-- name: MarkMessageDelivered :one
UPDATE messages
SET
    status = 'delivered',
    delivered_at = now()
WHERE channel = $1
  AND external_message_id = $2
RETURNING *;



-- name: MarkMessageRead :one
UPDATE messages
SET read_at = now()
WHERE channel = $1
  AND external_message_id = $2
RETURNING *;


-- name: MarkMessageFailed :one
UPDATE messages
SET status = 'failed'
WHERE id = $1
RETURNING *;


-- name: GetMessageByID :one
SELECT * FROM messages
WHERE id = $1
LIMIT 1;

-- name: ListMessagesByInvoice :many
SELECT * FROM messages
WHERE invoice_id = $1
ORDER BY created_at DESC;

-- name: ListMessagesByCustomer :many
SELECT * FROM messages
WHERE customer_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;


-- name: FetchApprovedMessages :many
SELECT * FROM messages
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;


-- name: GetMessageByExternalID :one
SELECT * FROM messages
WHERE channel = $1
  AND external_message_id = $2
LIMIT 1;


-- name: DeleteMessage :exec
DELETE FROM messages
WHERE id = $1;