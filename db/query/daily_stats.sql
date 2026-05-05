-- name: UpsertDailyStats :one
INSERT INTO daily_stats (
    user_id,
    date,
    total_outstanding,
    total_collected,
    invoices_unpaid,
    invoices_paid,
    messages_sent
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, date)
DO UPDATE SET
    total_outstanding = EXCLUDED.total_outstanding,
    total_collected   = EXCLUDED.total_collected,
    invoices_unpaid   = EXCLUDED.invoices_unpaid,
    invoices_paid     = EXCLUDED.invoices_paid,
    messages_sent     = EXCLUDED.messages_sent,
    updated_at        = now()
RETURNING *;

-- name: IncrementMessagesSent :exec
UPDATE daily_stats
SET messages_sent = messages_sent + $3
WHERE user_id = $1
  AND date = $2;

-- name: GetDailyStats :one
SELECT *
FROM daily_stats
WHERE user_id = $1
  AND date = $2;


-- name: ListDailyStatsRange :many
SELECT *
FROM daily_stats
WHERE user_id = $1
  AND date BETWEEN $2 AND $3
ORDER BY date ASC;

-- name: GetMonthlyStats :one
-- Powers the "Recovered this month" card in the v3 stats strip.
SELECT
    COALESCE(SUM(total_collected), 0)::BIGINT AS total_collected,
    COALESCE(SUM(messages_sent),   0)::INT    AS total_messages_sent
FROM daily_stats
WHERE user_id = $1
  AND date >= date_trunc('month', now())::DATE;

-- name: GetLatestDailyStats :one
SELECT *
FROM daily_stats
WHERE user_id = $1
ORDER BY date DESC
LIMIT 1;

-- name: DeleteStatsByDateRange :exec
DELETE FROM daily_stats
WHERE user_id = $1
  AND date BETWEEN $2 AND $3;