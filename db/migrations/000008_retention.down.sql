ALTER TABLE invoices DROP COLUMN IF EXISTS promised_at;

DROP TABLE IF EXISTS customer_events CASCADE;

DROP TYPE IF EXISTS customer_event_type;