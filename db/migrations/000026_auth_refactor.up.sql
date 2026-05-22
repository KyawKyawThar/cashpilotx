-- Sprint 0: Decouple team account identity from personal phone numbers.
-- Adds username + force_pin_change; makes phone optional.
-- Existing super_admin rows: phone stays, username stays NULL — valid per chk_user_identity.

ALTER TABLE users
    ADD COLUMN username         TEXT,
    ADD COLUMN force_pin_change BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE users
    ALTER COLUMN phone DROP NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT chk_user_identity
        CHECK (phone IS NOT NULL OR username IS NOT NULL);

-- Unique index: active username-based accounts only (allows reuse after deactivation)
CREATE UNIQUE INDEX uniq_active_username
    ON users(username)
    WHERE is_active = true AND username IS NOT NULL;
