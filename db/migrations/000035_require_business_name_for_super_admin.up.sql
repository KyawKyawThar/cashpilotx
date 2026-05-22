-- Enforce that every super_admin account has a business_name.
-- Backfill any existing NULL rows using the user's name as a safe fallback,
-- then add a CHECK constraint so no future row can violate the rule.

UPDATE users
    SET business_name = name
WHERE role = 'super_admin'
  AND business_name IS NULL;

ALTER TABLE users
    ADD CONSTRAINT chk_super_admin_business_name
        CHECK (role != 'super_admin' OR business_name IS NOT NULL);
