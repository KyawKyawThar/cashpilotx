ALTER TABLE invoices
    DROP COLUMN IF EXISTS return_reason,
    DROP COLUMN IF EXISTS return_items;
