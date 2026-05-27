-- Customer name and phone are no longer unique per user.
-- One real person can have multiple customer records (one per order).
DROP INDEX IF EXISTS uniq_customer_phone;
DROP INDEX IF EXISTS uniq_customer_name;
