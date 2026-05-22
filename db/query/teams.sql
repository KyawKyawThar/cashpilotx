-- name: CreateTeam :one
INSERT INTO teams (name, admin_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams
WHERE id = $1;

-- name: ListTeams :many
SELECT * FROM teams
ORDER BY created_at ASC;

-- name: UpdateTeamName :one
UPDATE teams
SET name = $2
WHERE id = $1
RETURNING *;

-- name: UpdateTeamAdmin :one
UPDATE teams
SET admin_id = $2
WHERE id = $1
RETURNING *;

-- name: DeleteTeam :exec
DELETE FROM teams WHERE id = $1;
