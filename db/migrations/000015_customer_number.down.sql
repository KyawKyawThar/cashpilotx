DROP INDEX IF EXISTS uniq_customer_number;
ALTER TABLE customers DROP COLUMN IF EXISTS customer_number;
ALTER TABLE users DROP COLUMN IF EXISTS customer_sequence;
