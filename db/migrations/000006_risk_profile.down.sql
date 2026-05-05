DROP TRIGGER IF EXISTS trg_risk_profiles_updated_at ON customer_risk_profiles;

DROP TABLE IF EXISTS customer_risk_profiles CASCADE;

DROP TYPE IF EXISTS late_fee_type_enum;
