DROP TRIGGER IF EXISTS trg_daily_stats_updated_at ON daily_stats;
DROP TRIGGER IF EXISTS trg_webhook_events_updated_at ON webhook_events;
DROP TRIGGER IF EXISTS trg_reminders_updated_at ON reminders;
DROP TRIGGER IF EXISTS trg_conversations_updated_at ON conversations;
DROP TRIGGER IF EXISTS trg_invoices_updated_at ON invoices;

DROP TABLE IF EXISTS daily_stats CASCADE;
DROP TABLE IF EXISTS webhook_events CASCADE;
DROP TABLE IF EXISTS reminders CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS conversations CASCADE;
DROP TABLE IF EXISTS payments CASCADE;
DROP TABLE IF EXISTS invoices CASCADE;

DROP TYPE IF EXISTS message_source;
DROP TYPE IF EXISTS message_status;
DROP TYPE IF EXISTS message_tone;
DROP TYPE IF EXISTS message_channel;
DROP TYPE IF EXISTS message_direction;