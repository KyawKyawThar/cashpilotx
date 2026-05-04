DROP TRIGGER IF EXISTS trg_customers_updated_at ON customers;

DROP TABLE IF EXISTS customer_contacts CASCADE;
DROP TABLE IF EXISTS customers CASCADE;

DROP TYPE IF EXISTS contact_platform;
