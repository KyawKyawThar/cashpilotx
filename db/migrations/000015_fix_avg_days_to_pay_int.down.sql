ALTER TABLE customer_risk_profiles
    ALTER COLUMN avg_days_to_pay TYPE NUMERIC(10,2) USING avg_days_to_pay::numeric(10,2);
