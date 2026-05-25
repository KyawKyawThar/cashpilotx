ALTER TABLE invoices DROP COLUMN IF EXISTS returned_at;

ALTER TABLE invoices
    DROP CONSTRAINT invoice_status_check,
    ADD CONSTRAINT invoice_status_check CHECK (
        status IN ('unpaid','partial','paid','cancelled')
    );
