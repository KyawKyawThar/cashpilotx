ALTER TABLE customer_risk_profiles
    ALTER COLUMN total_late_fees TYPE NUMERIC(14,2) USING total_late_fees::numeric / 100;
