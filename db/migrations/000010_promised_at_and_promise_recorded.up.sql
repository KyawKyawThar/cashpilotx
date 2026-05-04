-- promised_at on invoices: needed for "2d after promise" note in Payments page
-- and for ProcessInboundReplyTx to record the customer's committed date.
ALTER TABLE invoices
    ADD COLUMN promised_at TIMESTAMPTZ;

-- promise_recorded event type: powers the history timeline when an owner or the
-- system records a customer's payment promise (state → 'promised').
ALTER TYPE customer_event_type ADD VALUE IF NOT EXISTS 'promise_recorded' AFTER 'collection_state_changed';
