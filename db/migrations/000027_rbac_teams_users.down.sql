DROP INDEX IF EXISTS idx_teams_admin;
DROP INDEX IF EXISTS idx_users_team;
DROP INDEX IF EXISTS idx_users_org_owner;

ALTER TABLE users
    DROP COLUMN IF EXISTS org_owner_id,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS team_id,
    DROP COLUMN IF EXISTS role;

DROP TABLE IF EXISTS teams;
DROP TYPE IF EXISTS user_role;
