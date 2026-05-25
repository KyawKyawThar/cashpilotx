ALTER TABLE invoices
    ADD COLUMN return_reason TEXT,
    ADD COLUMN return_items  JSONB;
