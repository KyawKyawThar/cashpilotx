-- Sprint A: Teams table, user_role ENUM, and RBAC columns on users.
-- teams is created before adding team_id FK to users to avoid circular dep.

CREATE TABLE teams (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    admin_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE user_role AS ENUM ('super_admin', 'admin', 'team_leader', 'team_member');

ALTER TABLE users
    ADD COLUMN role         user_role NOT NULL DEFAULT 'super_admin',
    ADD COLUMN team_id      UUID REFERENCES teams(id) ON DELETE SET NULL,
    ADD COLUMN created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN org_owner_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- All existing users are super_admins who own their own org.
UPDATE users SET org_owner_id = id;

CREATE INDEX idx_users_org_owner ON users(org_owner_id);
CREATE INDEX idx_users_team      ON users(team_id);
CREATE INDEX idx_teams_admin     ON teams(admin_id);
