-- Sprint RBAC update: rename team_leader→cs_team, team_member→sales, add accountant.
-- Pattern: convert to TEXT, migrate, drop old enum, create new enum, restore type.

-- ── 1. Release the enum lock on both tables ───────────────────────────────────
ALTER TABLE users          ALTER COLUMN role TYPE TEXT;
ALTER TABLE role_permissions ALTER COLUMN role TYPE TEXT;

-- Drop the default before dropping the type; the default expression
-- still references user_role even after the column is cast to TEXT.
ALTER TABLE users ALTER COLUMN role DROP DEFAULT;

-- ── 2. Migrate existing role data ─────────────────────────────────────────────
UPDATE users          SET role = 'cs_team' WHERE role = 'team_leader';
UPDATE users          SET role = 'sales'   WHERE role = 'team_member';
UPDATE role_permissions SET role = 'cs_team' WHERE role = 'team_leader';
UPDATE role_permissions SET role = 'sales'   WHERE role = 'team_member';

-- ── 3. Remove stale role_permissions rows for old roles ───────────────────────
DELETE FROM role_permissions WHERE role IN ('team_leader', 'team_member');

-- ── 4. Rename / add permission catalog entries for team management ────────────
UPDATE permissions SET action = 'create_cs_team',  description = 'Create CS Team accounts'
    WHERE resource = 'team' AND action = 'create_team_leader';

UPDATE permissions SET action = 'create_sales',    description = 'Create Sales accounts'
    WHERE resource = 'team' AND action = 'create_team_member';

INSERT INTO permissions (resource, action, description)
    VALUES ('team', 'create_accountant', 'Create Accountant accounts')
    ON CONFLICT (resource, action, qualifier) DO NOTHING;

-- ── 5. Swap the enum type ─────────────────────────────────────────────────────
DROP TYPE user_role;
CREATE TYPE user_role AS ENUM ('super_admin', 'admin', 'accountant', 'cs_team', 'sales');

-- ── 6. Restore strongly-typed columns ─────────────────────────────────────────
ALTER TABLE users          ALTER COLUMN role TYPE user_role USING role::user_role;
ALTER TABLE role_permissions ALTER COLUMN role TYPE user_role USING role::user_role;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'super_admin'::user_role;

-- ── 7. Seed role_permissions for accountant ───────────────────────────────────
-- Accountant: everything except create admin/cs_team accounts and grant_permission
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'accountant', id, 'global'
FROM permissions
WHERE NOT (resource = 'team' AND action IN ('create_admin', 'create_cs_team', 'grant_permission'))
ON CONFLICT (role, permission_id) DO NOTHING;

-- ── 8. Seed role_permissions for cs_team ─────────────────────────────────────
-- CS Team: all except payment:record/void, recovery_queue, cost price, manage actions
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'cs_team', id, 'global'
FROM permissions
WHERE (resource = 'invoice'          AND action IN ('view','view_history','create','edit','delete','export'))
   OR (resource = 'customer'         AND action IN ('view','create','edit','delete','view_risk_profile','set_credit_limit'))
   OR (resource = 'payment'          AND action = 'view')
   OR (resource = 'reminder'         AND action IN ('view','create','cancel'))
   OR (resource = 'message'          AND action IN ('view','send'))
   OR (resource = 'audit_log'        AND action = 'view')
   OR (resource = 'report'           AND action IN ('view','export'))
   OR (resource = 'product'          AND action = 'view_selling')
   OR (resource = 'channel_config'   AND action = 'view')
   OR (resource = 'delivery_service' AND action = 'view')
   OR (resource = 'team'             AND action = 'view')
ON CONFLICT (role, permission_id) DO NOTHING;

-- ── 9. Seed role_permissions for sales ───────────────────────────────────────
-- Sales: can only create and view customers (own scope)
INSERT INTO role_permissions (role, permission_id, scope)
SELECT 'sales', id, 'own'
FROM permissions
WHERE (resource = 'customer'         AND action IN ('view','create'))
   OR (resource = 'product'          AND action = 'view_selling')
   OR (resource = 'delivery_service' AND action = 'view')
ON CONFLICT (role, permission_id) DO NOTHING;
