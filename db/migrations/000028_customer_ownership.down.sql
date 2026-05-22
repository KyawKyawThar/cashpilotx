DROP INDEX IF EXISTS idx_customers_assigned;

ALTER TABLE customers
    DROP COLUMN IF EXISTS credit_limit_approved_by,
    DROP COLUMN IF EXISTS assigned_to;
