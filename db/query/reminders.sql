-- name: CreateReminder :one
INSERT INTO reminders (
    user_id,
    invoice_id,
    customer_id,
    scheduled_at,
    tone,
    channel,
    max_attempts
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: GetReminder :one
SELECT * FROM reminders
WHERE id = $1;

-- name: GetActiveReminderByInvoice :one
-- Used by FireReminderTx to check for an already-active reminder before creating a new one.
SELECT * FROM reminders
WHERE invoice_id = $1
  AND status IN ('pending', 'processing')
LIMIT 1;

-- name: ListRemindersByInvoice :many
SELECT * FROM reminders
WHERE invoice_id = $1
ORDER BY created_at DESC;

-- name: PickAndMarkReminders :many
-- Worker pickup: atomically claim pending/retry reminders for processing.
WITH picked AS (
    SELECT id
    FROM reminders
    WHERE status = 'pending'
      AND (
            (next_retry_at IS NULL     AND scheduled_at  <= now())
            OR
            (next_retry_at IS NOT NULL AND next_retry_at <= now())
          )
    ORDER BY COALESCE(next_retry_at, scheduled_at), id
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
UPDATE reminders r
SET status     = 'processing',
    updated_at = now()
FROM picked
WHERE r.id = picked.id
RETURNING r.*;

-- name: MarkReminderSent :exec
UPDATE reminders
SET status              = 'sent',
    sent_at             = now(),
    external_message_id = $2
WHERE id = $1
  AND status = 'processing';

-- name: MarkReminderFailed :exec
UPDATE reminders
SET status         = CASE
                         WHEN attempt_number + 1 >= max_attempts THEN 'failed'
                         ELSE 'pending'
                     END,
    attempt_number = attempt_number + 1,
    last_error     = $2,
    next_retry_at  = $3
WHERE id = $1
  AND status = 'processing';

-- name: CancelReminder :exec
UPDATE reminders
SET status       = 'cancelled',
    cancelled_at = now()
WHERE id = $1
  AND status IN ('pending','processing','failed');

-- name: RescheduleReminder :exec
UPDATE reminders
SET status        = 'pending',
    scheduled_at  = $2,
    next_retry_at = NULL
WHERE id = $1
  AND status IN ('failed','pending');

-- name: ListRemindersByUser :many
SELECT *
FROM reminders
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CancelRemindersForInvoice :exec
UPDATE reminders
SET status       = 'cancelled',
    cancelled_at = now(),
    updated_at   = now()
WHERE invoice_id = $1
  AND status IN ('pending', 'processing');
