DROP TRIGGER IF EXISTS trg_exchange_rates_updated_at ON exchange_rates;

DROP TABLE IF EXISTS exchange_rates CASCADE;

DROP TYPE IF EXISTS rate_source;
