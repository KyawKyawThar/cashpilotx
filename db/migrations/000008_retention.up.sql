-- =========================
-- CUSTOMER EVENTS
-- Required for v3 timeline / History modal.
-- All transactions that touch invoices must emit a row here.
-- Phase 2 tables (customer_insights, daily_customer_stats, weekly_summaries) removed.
-- =========================
CREATE TYPE customer_event_type AS ENUM (
  'customer_created',
  'invoice_created',
  'payment_received',
  'reminder_sent',
  'message_sent',
  'reply_received',
  'collection_state_changed',
  'overdue_escalated',
  'risk_score_changed',
  'channel_connected'
);

CREATE TABLE customer_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    customer_id   UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    invoice_id    UUID REFERENCES invoices(id) ON DELETE CASCADE,

    event_type    customer_event_type NOT NULL,
    event_value   JSONB,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_customer_events_lookup
ON customer_events(user_id, customer_id, created_at DESC);

CREATE INDEX idx_customer_events_invoice
ON customer_events(invoice_id, created_at DESC)
WHERE invoice_id IS NOT NULL;

CREATE INDEX idx_customer_events_type
ON customer_events(event_type);

CREATE INDEX idx_events_recent
ON customer_events(user_id, created_at DESC);
