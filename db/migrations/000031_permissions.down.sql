DROP INDEX IF EXISTS idx_user_grants_permission;
DROP INDEX IF EXISTS idx_user_grants_user;
DROP INDEX IF EXISTS idx_role_permissions_role;

DROP TABLE IF EXISTS user_permission_grants;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TYPE IF EXISTS data_scope;
