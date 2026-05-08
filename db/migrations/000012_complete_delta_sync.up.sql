ALTER TABLE payments
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_trigger
    WHERE tgname = 'trg_payments_updated_at'
  ) THEN
    CREATE TRIGGER trg_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_invoices_updated
ON invoices(user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_customers_updated
ON customers(user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_reminders_updated
ON reminders(user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_payments_updated
ON payments(user_id, updated_at DESC);
