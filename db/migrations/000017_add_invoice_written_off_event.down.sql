-- Postgres cannot remove an enum value directly.
-- Recreate the type without 'invoice_written_off'.
DELETE FROM customer_events WHERE event_type = 'invoice_written_off';

ALTER TYPE customer_event_type RENAME TO customer_event_type_old;

CREATE TYPE customer_event_type AS ENUM (
  'customer_created',
  'invoice_created',
  'invoice_canceled',
  'payment_received',
  'reminder_sent',
  'message_sent',
  'reply_received',
  'collection_state_changed',
  'promise_recorded',
  'overdue_escalated',
  'risk_score_changed',
  'channel_connected'
);

ALTER TABLE customer_events
  ALTER COLUMN event_type TYPE customer_event_type
  USING event_type::text::customer_event_type;

DROP TYPE customer_event_type_old;
