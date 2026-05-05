-- name: CreateWebhookEvent :one
WITH ins AS (
    INSERT INTO webhook_events (
        provider,
        event_id,
        user_id,
        payload,
        headers,
        signature
    )
    VALUES ($1, $2, $3, $4, $5, $6)
    ON CONFLICT (provider, event_id) DO NOTHING
    RETURNING *
)
SELECT * FROM ins
UNION
SELECT * FROM webhook_events
WHERE provider = $1 AND event_id = $2
LIMIT 1;

-- name: GetWebhookEvent :one
SELECT *
FROM webhook_events
WHERE provider = $1
  AND event_id = $2
LIMIT 1;

-- name: PickWebhookEvents :many
WITH picked AS (
    SELECT id
    FROM webhook_events
    WHERE status = 'pending'
      AND (next_retry_at IS NULL OR next_retry_at <= now())
    ORDER BY received_at
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
UPDATE webhook_events w
SET status = 'processing'
FROM picked
WHERE w.id = picked.id
RETURNING w.*;

-- name: MarkWebhookProcessed :exec
UPDATE webhook_events
SET status = 'processed',
    processed_at = now()
WHERE id = $1
  AND status = 'processing';


-- name: MarkWebhookFailed :exec
UPDATE webhook_events
SET status = 'pending',
    attempt_count = attempt_count + 1,
    last_error = $2,
    next_retry_at = $3
WHERE id = $1
  AND status = 'processing';


-- name: MarkWebhookDead :exec
UPDATE webhook_events
SET status = 'failed',
    last_error = $2
WHERE id = $1
  AND status = 'processing';

-- name: RequeueWebhookEvent :exec
UPDATE webhook_events
SET status = 'pending',
    next_retry_at = NULL
WHERE id = $1
  AND status IN ('failed','processed');


-- name: ListFailedWebhookEvents :many
SELECT *
FROM webhook_events
WHERE status = 'failed'
ORDER BY updated_at DESC
LIMIT $1 OFFSET $2;


-- name: DeleteOldProcessedWebhookEvents :exec
DELETE FROM webhook_events
WHERE status = 'processed'
  AND processed_at < now() - INTERVAL '30 days';