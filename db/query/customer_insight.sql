-- name: CreateCustomerInsight :one
INSERT INTO customer_insights (
    user_id,
    customer_id,
    insight_type,
    insight_text,
    score,
    delta_score,
    metadata,
    valid_from,
    valid_to,
    is_current
) VALUES (
    $1, $2, $3, $4, $5, $6,
    COALESCE($7::jsonb, '{}'::jsonb),
    COALESCE($8::timestamptz, now()),
    $9,
    COALESCE($10::boolean, true)
)
RETURNING *;


-- name: GetCustomerInsightByID :one
SELECT * FROM customer_insights
WHERE id = $1;


-- name: GetCurrentInsight :one
SELECT * FROM customer_insights
WHERE user_id = $1
  AND customer_id = $2
  AND insight_type = $3
  AND is_current = true
LIMIT 1;


-- name: ListCurrentInsightsByCustomer :many
SELECT * FROM customer_insights
WHERE user_id = $1
  AND customer_id = $2
  AND is_current = true
ORDER BY insight_type;


-- name: ListInsightsHistory :many
SELECT * FROM customer_insights
WHERE user_id = $1
  AND customer_id = $2
  AND insight_type = $3
ORDER BY valid_from DESC;


-- name: CloseCurrentInsight :exec
UPDATE customer_insights
SET
    is_current = false,
    valid_to = COALESCE($4, now())
WHERE user_id = $1
  AND customer_id = $2
  AND insight_type = $3
  AND is_current = true;


-- name: ReplaceInsightAtomic :one
WITH closed AS (
    UPDATE customer_insights
    SET
        is_current = false,
        valid_to = now()
    WHERE customer_insights.user_id = $1
      AND customer_insights.customer_id = $2
      AND customer_insights.insight_type = $3
      AND customer_insights.is_current = true
    RETURNING 1
),
inserted AS (
    INSERT INTO customer_insights (
        user_id,
        customer_id,
        insight_type,
        insight_text,
        score,
        delta_score,
        metadata,
        valid_from,
        is_current
    )
    SELECT
        $1, $2, $3, $4, $5, $6,
        COALESCE($7::jsonb, '{}'::jsonb),
        now(),
        true
    FROM (SELECT count(*) FROM closed) AS ensure_closed
    RETURNING *
)
SELECT * FROM inserted;


-- name: DeleteInsightByID :exec
DELETE FROM customer_insights
WHERE id = $1;


-- name: GetActiveInsightsByUser :many
SELECT * FROM customer_insights
WHERE user_id = $1
  AND is_current = true
ORDER BY created_at DESC;


-- name: GetInsightsExpiringSoon :many
SELECT * FROM customer_insights
WHERE user_id = $1
  AND is_current = true
  AND valid_to IS NOT NULL
  AND valid_to <= now() + interval '3 days'
ORDER BY valid_to ASC;
