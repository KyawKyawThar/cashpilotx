DROP INDEX IF EXISTS idx_delivery_services_active;

ALTER TABLE delivery_services
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS price;
