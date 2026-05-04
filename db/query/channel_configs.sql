-- name: UpsertChannelConfig :one
INSERT INTO channel_configs (
    user_id,
    channel,
    config_encrypted,
    config_nonce,
    config_tag,
    key_id,
    config_version,
    is_active,
    status
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (user_id, channel) WHERE deleted_at IS NULL
DO UPDATE SET
    config_encrypted = EXCLUDED.config_encrypted,
    config_nonce     = EXCLUDED.config_nonce,
    config_tag       = EXCLUDED.config_tag,
    key_id           = EXCLUDED.key_id,
    config_version   = channel_configs.config_version + 1,
    is_active        = EXCLUDED.is_active,
    status           = EXCLUDED.status,
    updated_at       = now()
RETURNING *;

-- name: GetActiveChannelConfig :one
SELECT *
FROM channel_configs
WHERE user_id = $1
  AND channel = $2
  AND is_active = TRUE
  AND deleted_at IS NULL;


-- name: ListChannelConfigs :many
SELECT *
FROM channel_configs
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC;


-- name: MarkChannelConnected :exec
UPDATE channel_configs
SET status = 'connected',
    last_connected_at = now(),
    last_error = NULL
WHERE id = $1;



-- name: MarkChannelError :exec
UPDATE channel_configs
SET status = 'error',
    last_error = $2
WHERE id = $1;

-- name: DeleteChannelConfig :exec
UPDATE channel_configs
SET deleted_at = now(),
    is_active = FALSE
WHERE id = $1;

-- name: SetChannelActive :exec
UPDATE channel_configs
SET is_active = $2
WHERE id = $1
  AND deleted_at IS NULL;