DROP TRIGGER IF EXISTS trg_users_updated_at ON users;

DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Drop last: function is shared by all migrations above this one
DROP FUNCTION IF EXISTS set_updated_at();
