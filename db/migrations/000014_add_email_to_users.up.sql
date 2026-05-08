ALTER TABLE users ADD COLUMN email TEXT;

CREATE UNIQUE INDEX uniq_active_email
    ON users(email)
    WHERE is_active = true AND email IS NOT NULL;
