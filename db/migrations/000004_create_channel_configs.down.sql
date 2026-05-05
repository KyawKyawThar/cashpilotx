DROP TRIGGER IF EXISTS trg_channel_configs_updated_at ON channel_configs;

DROP TABLE IF EXISTS channel_configs CASCADE;

DROP TYPE IF EXISTS channel_type;
