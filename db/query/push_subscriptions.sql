-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, endpoint) DO UPDATE
    SET p256dh     = EXCLUDED.p256dh,
        auth       = EXCLUDED.auth,
        created_at = now()
RETURNING *;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions
WHERE user_id = $1
  AND endpoint = $2;

-- name: ListPushSubscriptionsForUser :many
SELECT id, user_id, endpoint, p256dh, auth, created_at
FROM push_subscriptions
WHERE user_id = $1
ORDER BY created_at ASC;
