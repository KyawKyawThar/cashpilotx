-- name: GetPermissionByResourceAction :one
SELECT * FROM permissions
WHERE resource = $1 AND action = $2 AND qualifier = ''
LIMIT 1;

-- name: CreatePermissionGrant :one
INSERT INTO user_permission_grants (user_id, permission_id, scope, scope_team_id, granted_by, reason, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPermissionGrants :many
SELECT * FROM user_permission_grants
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListPermissionGrantsWithPermission :many
SELECT
    g.id, g.user_id, g.permission_id, g.scope, g.scope_team_id,
    g.granted_by, g.reason, g.expires_at, g.created_at,
    p.resource, p.action
FROM user_permission_grants g
JOIN permissions p ON p.id = g.permission_id
WHERE g.user_id = $1
ORDER BY g.created_at DESC;

-- name: GetPermissionGrant :one
SELECT * FROM user_permission_grants
WHERE id = $1 AND user_id = $2
LIMIT 1;

-- name: DeletePermissionGrant :exec
DELETE FROM user_permission_grants
WHERE id = $1 AND user_id = $2;
