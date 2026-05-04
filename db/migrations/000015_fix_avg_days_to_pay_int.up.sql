ALTER TABLE customer_risk_profiles
    ALTER COLUMN avg_days_to_pay TYPE INT USING avg_days_to_pay::numeric::int;
