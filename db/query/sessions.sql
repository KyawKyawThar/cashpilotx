-- name: CreateSession :one
INSERT INTO sessions (
    user_id,
    refresh_token,
    user_agent,
    ip_address,
    device_id,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;



-- name: GetSessionByRefreshToken :one
SELECT *
FROM sessions
WHERE refresh_token = $1
  AND expires_at > now()
LIMIT 1;

-- name: ListUserSessions :many
SELECT *
FROM sessions
WHERE user_id = $1
  AND expires_at > now()
ORDER BY created_at ASC;



-- name: DeleteSessionByID :one
DELETE FROM sessions
WHERE id = $1
RETURNING id;

-- name: DeleteSessionByRefreshToken :one
DELETE FROM sessions
WHERE refresh_token = $1
RETURNING id;

-- name: DeleteAllUserSessions :exec
DELETE FROM sessions
WHERE user_id = $1;



-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= now();