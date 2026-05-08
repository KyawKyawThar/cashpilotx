DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
 
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS users CASCADE;
 
-- Drop last: function may be shared by other triggers
DROP FUNCTION IF EXISTS set_updated_at();