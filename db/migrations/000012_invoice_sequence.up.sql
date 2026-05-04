-- invoice_sequence: per-user atomic counter for human-readable invoice numbers.
-- CreateInvoiceTx increments this with UPDATE ... RETURNING and formats the
-- external_id as "INV-{year}-{seq:03d}" when the caller omits external_id.
ALTER TABLE users
    ADD COLUMN invoice_sequence INT NOT NULL DEFAULT 0
    CHECK (invoice_sequence >= 0);
