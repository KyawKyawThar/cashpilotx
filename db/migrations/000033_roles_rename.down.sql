-- Reverse 000033: restore original team_leader / team_member enum values.

ALTER TABLE users          ALTER COLUMN role TYPE TEXT;
ALTER TABLE role_permissions ALTER COLUMN role TYPE TEXT;

-- Drop the column default before dropping the type; the default expression
-- still references user_role even after the column is cast to TEXT.
ALTER TABLE users ALTER COLUMN role DROP DEFAULT;

UPDATE users          SET role = 'admin'       WHERE role = 'accountant';
UPDATE users          SET role = 'team_leader' WHERE role = 'cs_team';
UPDATE users          SET role = 'team_member' WHERE role = 'sales';
UPDATE role_permissions SET role = 'team_leader' WHERE role = 'cs_team';
UPDATE role_permissions SET role = 'team_member' WHERE role = 'sales';

DELETE FROM role_permissions WHERE role IN ('accountant', 'cs_team', 'sales');

UPDATE permissions SET action = 'create_team_leader', description = 'Create team leader accounts'
    WHERE resource = 'team' AND action = 'create_cs_team';
UPDATE permissions SET action = 'create_team_member', description = 'Create team member accounts'
    WHERE resource = 'team' AND action = 'create_sales';
DELETE FROM permissions WHERE resource = 'team' AND action = 'create_accountant';

DROP TYPE user_role;
CREATE TYPE user_role AS ENUM ('super_admin', 'admin', 'team_leader', 'team_member');

ALTER TABLE users          ALTER COLUMN role TYPE user_role USING role::user_role;
ALTER TABLE role_permissions ALTER COLUMN role TYPE user_role USING role::user_role;

ALTER TABLE users ALTER COLUMN role SET DEFAULT 'super_admin'::user_role;
