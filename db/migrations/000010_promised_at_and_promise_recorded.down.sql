ALTER TABLE invoices DROP COLUMN IF EXISTS promised_at;

-- PostgreSQL does not support removing ENUM values directly.
-- Recreate the type without 'promise_recorded'.
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
    'overdue_escalated',
    'risk_score_changed',
    'channel_connected',
    'invoice_written_off'
);

ALTER TABLE customer_events
    ALTER COLUMN event_type TYPE customer_event_type
    USING event_type::text::customer_event_type;

DROP TYPE customer_event_type_old;
