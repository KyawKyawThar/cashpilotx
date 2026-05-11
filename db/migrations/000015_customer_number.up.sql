-- Per-user atomic counter for human-readable customer numbers.
-- CreateCustomerTx increments this with UPDATE ... RETURNING and formats
-- customer_number as "C-{seq:04d}" (e.g. C-0001, C-0042).
ALTER TABLE users
    ADD COLUMN customer_sequence INT NOT NULL DEFAULT 0 CHECK (customer_sequence >= 0);

-- Human-readable display number assigned at creation time.
-- NULL for customers that pre-date this migration.
ALTER TABLE customers
    ADD COLUMN customer_number TEXT;

-- Ensures uniqueness per user while allowing NULLs from pre-migration rows.
CREATE UNIQUE INDEX uniq_customer_number
    ON customers(user_id, customer_number)
    WHERE customer_number IS NOT NULL;
