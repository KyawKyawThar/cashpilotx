-- Clear any non-UUID values stored from the old text-based approach
UPDATE invoices SET assigned_to = NULL WHERE assigned_to IS NOT NULL;

-- Change column type from TEXT to UUID with FK referencing users
ALTER TABLE invoices
    ALTER COLUMN assigned_to TYPE uuid USING assigned_to::uuid;

ALTER TABLE invoices
    ADD CONSTRAINT fk_invoices_assigned_to
    FOREIGN KEY (assigned_to) REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_invoices_assigned_to ON invoices(assigned_to) WHERE assigned_to IS NOT NULL;
