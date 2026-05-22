-- Sprint A: Permissions catalog — permissions, role_permissions, user_permission_grants.
-- qualifier is TEXT NOT NULL DEFAULT '' so UNIQUE (resource, action, qualifier) works cleanly.

CREATE TYPE data_scope AS ENUM ('global', 'team', 'own');

CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource    TEXT NOT NULL,
    action      TEXT NOT NULL,
    qualifier   TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    CONSTRAINT permissions_unique UNIQUE (resource, action, qualifier)
);

CREATE TABLE role_permissions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role          user_role NOT NULL,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    scope         data_scope NOT NULL,
    CONSTRAINT role_permissions_unique UNIQUE (role, permission_id)
);

-- Per-user grant exceptions (only super_admin can write rows here).
CREATE TABLE user_permission_grants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    scope         data_scope NOT NULL,
    scope_team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    granted_by    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason        TEXT,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_role_permissions_role  ON role_permissions(role);
CREATE INDEX idx_user_grants_user       ON user_permission_grants(user_id);
CREATE INDEX idx_user_grants_permission ON user_permission_grants(permission_id);
