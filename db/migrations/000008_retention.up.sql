-- =========================
-- CUSTOMER EVENTS
-- =========================

-- Full enum declared upfront — no incremental ALTERs needed
CREATE TYPE customer_event_type AS ENUM (
    'customer_created',
    'invoice_created',
    'invoice_canceled',       -- added in 000009
    'invoice_written_off',    -- added in 000017
    'payment_received',
    'reminder_sent',
    'message_sent',
    'reply_received',
    'collection_state_changed',
    'promise_recorded',       -- added in 000010
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

-- From 000010: needed for "2d after promise" note and ProcessInboundReplyTx
ALTER TABLE invoices
    ADD COLUMN promised_at TIMESTAMPTZ;