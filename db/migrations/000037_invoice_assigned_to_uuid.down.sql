DROP INDEX IF EXISTS idx_invoices_assigned_to;

ALTER TABLE invoices
    DROP CONSTRAINT IF EXISTS fk_invoices_assigned_to;

ALTER TABLE invoices
    ALTER COLUMN assigned_to TYPE text USING assigned_to::text;
