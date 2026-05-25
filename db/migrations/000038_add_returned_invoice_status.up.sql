ALTER TABLE invoices
    DROP CONSTRAINT invoice_status_check,
    ADD CONSTRAINT invoice_status_check CHECK (
        status IN ('unpaid','partial','paid','cancelled','returned')
    );

ALTER TABLE invoices ADD COLUMN returned_at TIMESTAMPTZ;

ALTER TYPE customer_event_type ADD VALUE IF NOT EXISTS 'invoice_returned' AFTER 'invoice_canceled';
