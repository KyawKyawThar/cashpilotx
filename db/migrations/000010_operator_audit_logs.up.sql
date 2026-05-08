-- ═══════════════════════════════════════════════════════
-- ENUMS
-- ═══════════════════════════════════════════════════════

CREATE TYPE audit_action_type AS ENUM (
    'message_copied',
    'message_marked_sent',
    'payment_recorded',
    'invoice_created',
    'reminder_scheduled',
    'collection_state_changed',
    'action_reverted'
);

CREATE TYPE audit_entity_type AS ENUM (
    'invoice',
    'payment',
    'message',
    'reminder',
    'customer',
    'system'
);



-- ═══════════════════════════════════════════════════════
-- TABLE
-- ═══════════════════════════════════════════════════════

CREATE TABLE operator_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- multi-tenant isolation
    user_id UUID NOT NULL
        REFERENCES users(id)
        ON DELETE CASCADE,

    -- who performed action
    performed_by UUID NOT NULL
        REFERENCES users(id),

    -- action
    action_type audit_action_type NOT NULL,

    -- logical grouping
    entity_type audit_entity_type NOT NULL DEFAULT 'invoice',

    -- display labels
    action_label TEXT NOT NULL,
    detail TEXT NOT NULL,

    -- denormalized references
    invoice_id UUID
        REFERENCES invoices(id),

    invoice_number TEXT,

    customer_id UUID
        REFERENCES customers(id),

    payment_id UUID
        REFERENCES payments(id),

    reminder_id UUID
        REFERENCES reminders(id),

    message_id UUID
        REFERENCES messages(id),

    -- revert system
    is_reverted BOOLEAN NOT NULL DEFAULT FALSE,

    reverted_by UUID
        REFERENCES operator_audit_log(id),

    reverted_at TIMESTAMPTZ,

    -- structured metadata
    metadata JSONB NOT NULL DEFAULT '{}',

    -- offline replay/idempotency
    request_id TEXT,

    -- timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- constraints
    CONSTRAINT chk_audit_label_not_empty
        CHECK (length(trim(action_label)) > 0),

    CONSTRAINT chk_audit_detail_not_empty
        CHECK (length(trim(detail)) > 0)
);



-- ═══════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════

CREATE INDEX idx_audit_user_date
ON operator_audit_log(user_id, created_at DESC);

CREATE INDEX idx_audit_type
ON operator_audit_log(user_id, action_type, created_at DESC);

CREATE INDEX idx_audit_invoice
ON operator_audit_log(invoice_id, created_at DESC);

CREATE INDEX idx_audit_customer
ON operator_audit_log(customer_id, created_at DESC);

CREATE INDEX idx_audit_payment
ON operator_audit_log(payment_id, created_at DESC);

CREATE INDEX idx_audit_message
ON operator_audit_log(message_id, created_at DESC);

CREATE INDEX idx_audit_reminder
ON operator_audit_log(reminder_id, created_at DESC);

CREATE INDEX idx_audit_entity
ON operator_audit_log(user_id, entity_type, created_at DESC);

CREATE INDEX idx_audit_reverted
ON operator_audit_log(user_id, created_at DESC)
WHERE is_reverted = TRUE;

CREATE INDEX idx_audit_request_id
ON operator_audit_log(user_id, request_id)
WHERE request_id IS NOT NULL;

CREATE INDEX idx_audit_metadata_gin
ON operator_audit_log
USING GIN(metadata);



-- ═══════════════════════════════════════════════════════
-- IMMUTABLE PROTECTION
-- append-only except revert fields
-- ═══════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION prevent_audit_log_mutation()
RETURNS TRIGGER AS $$
BEGIN

    IF (
        OLD.is_reverted IS DISTINCT FROM NEW.is_reverted
        OR OLD.reverted_by IS DISTINCT FROM NEW.reverted_by
        OR OLD.reverted_at IS DISTINCT FROM NEW.reverted_at
    ) THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'operator_audit_log is append-only';

END;
$$ LANGUAGE plpgsql;


CREATE TRIGGER trg_prevent_audit_log_mutation
BEFORE UPDATE ON operator_audit_log
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_log_mutation();



-- ═══════════════════════════════════════════════════════
-- OPTIONAL:
-- prevent hard delete in production
-- ═══════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION prevent_audit_log_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'operator_audit_log rows cannot be deleted';
END;
$$ LANGUAGE plpgsql;


CREATE TRIGGER trg_prevent_audit_log_delete
BEFORE DELETE ON operator_audit_log
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_log_delete();