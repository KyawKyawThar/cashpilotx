-- Prevent duplicate customers per user.
-- Phone uniqueness: same phone = same person — hard constraint.
-- Name uniqueness: handled at service level for better error messages.
CREATE UNIQUE INDEX uniq_customer_phone
    ON customers (user_id, normalized_phone)
    WHERE normalized_phone IS NOT NULL;

CREATE UNIQUE INDEX uniq_customer_name
    ON customers (user_id, normalized_name);
