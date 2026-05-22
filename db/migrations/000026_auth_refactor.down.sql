ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_user_identity;

DROP INDEX IF EXISTS uniq_active_username;

-- Team accounts (username-only login) cannot exist in the pre-refactor schema.
-- Remove them before restoring the NOT NULL constraint on phone.
DELETE FROM users WHERE phone IS NULL;

ALTER TABLE users
    ALTER COLUMN phone SET NOT NULL;

ALTER TABLE users
    DROP COLUMN IF EXISTS username,
    DROP COLUMN IF EXISTS force_pin_change;
