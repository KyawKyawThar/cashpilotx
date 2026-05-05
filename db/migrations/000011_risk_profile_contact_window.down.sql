ALTER TABLE customer_risk_profiles
    DROP COLUMN IF EXISTS best_contact_day,
    DROP COLUMN IF EXISTS best_contact_time,
    DROP COLUMN IF EXISTS best_contact_window;
