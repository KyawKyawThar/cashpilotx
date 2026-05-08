DROP INDEX IF EXISTS idx_payments_updated;
DROP INDEX IF EXISTS idx_reminders_updated;
DROP INDEX IF EXISTS idx_customers_updated;
DROP INDEX IF EXISTS idx_invoices_updated;

DROP TRIGGER IF EXISTS trg_payments_updated_at ON payments;
ALTER TABLE payments DROP COLUMN IF EXISTS updated_at;
