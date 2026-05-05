ALTER TABLE customer_risk_profiles
    ALTER COLUMN total_late_fees TYPE BIGINT USING (total_late_fees * 100)::bigint;
