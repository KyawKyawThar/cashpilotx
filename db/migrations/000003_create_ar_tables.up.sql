CREATE TYPE "message_direction" AS ENUM (
  'inbound',
  'outbound'
);

CREATE TYPE "message_channel" AS ENUM (
  'line',
  'messenger',
  'wechat',
  'telegram',
  'external'
);

CREATE TYPE "message_tone" AS ENUM (
  'friendly',
  'neutral',
  'firm',
  'strict'
);

CREATE TYPE "message_status" AS ENUM (
  'draft',
  'pending',
  'sent',
  'delivered',
  'read',
  'failed'
);

CREATE TYPE "message_source" AS ENUM (
  'ai',
  'human',
  'system'
);

-- =========================
-- INVOICES
-- =========================
CREATE TABLE invoices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,

    amount          BIGINT NOT NULL,
    currency        CHAR(3) NOT NULL,

    paid_amount     BIGINT NOT NULL DEFAULT 0,

    -- Set automatically when status transitions to 'paid' (used for avg_days_to_pay in risk scoring)
    paid_at         TIMESTAMPTZ,

    description     TEXT,
    due_date        DATE NOT NULL,

    status          TEXT NOT NULL DEFAULT 'unpaid',

    source          TEXT,
    photo_url       TEXT,

    external_id     TEXT,
    issued_at       TIMESTAMPTZ DEFAULT now(),

    -- Line items from Add Invoice form or OCR pipeline
    -- [{name, qty, unit_price, total, currency}], amounts in minor units
    items           JSONB NOT NULL DEFAULT '[]',

    -- Collection lifecycle state (independent of financial status)
    -- pending → reminded → no_response → negotiating → promised → paid | defaulted
    collection_state    TEXT NOT NULL DEFAULT 'pending',
    last_contacted_at   TIMESTAMPTZ,
    last_response_at    TIMESTAMPTZ,
    follow_up_count     INT NOT NULL DEFAULT 0,

    -- AI confidence scoring for Recovery Queue ranking (written by RefreshRecoveryQueueTx)
    -- confidence_factors: [{label, signal, color: green|amber|red}]
    confidence_score        FLOAT,
    confidence_factors      JSONB,
    outcome_text            TEXT,
    outcome_timeframe       TEXT,
    confidence_computed_at  TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT invoice_status_check CHECK (
        status IN ('unpaid','partial','paid','cancelled')
    ),
    CONSTRAINT invoice_collection_state_check CHECK (
        collection_state IN (
            'pending','reminded','no_response',
            'negotiating','promised','paid','defaulted'
        )
    ),
    CONSTRAINT invoice_amount_check  CHECK (amount > 0),
    CONSTRAINT invoice_paid_check    CHECK (paid_amount >= 0),
    CONSTRAINT invoice_no_overpay    CHECK (paid_amount <= amount),
    CONSTRAINT invoice_currency_check CHECK (char_length(currency) = 3),
    CONSTRAINT invoice_follow_up_check CHECK (follow_up_count >= 0)
);

CREATE INDEX idx_invoices_user_created ON invoices(user_id, created_at DESC);
CREATE INDEX idx_invoices_user_status  ON invoices(user_id, status);
CREATE INDEX idx_invoices_user_due     ON invoices(user_id, due_date);
CREATE INDEX idx_invoices_customer     ON invoices(user_id, customer_id);

-- Worker: pick overdue unpaid invoices for reminder scheduling
CREATE INDEX idx_invoices_overdue
ON invoices(user_id, due_date)
WHERE status IN ('unpaid','partial');

-- Recovery Queue: "who hasn't responded in N days?" + ranking by confidence
CREATE INDEX idx_invoices_collection_state
ON invoices(user_id, collection_state, last_contacted_at)
WHERE status NOT IN ('paid','cancelled');

CREATE INDEX idx_invoices_confidence
ON invoices(user_id, confidence_score DESC)
WHERE status NOT IN ('paid','cancelled');

-- Idempotency
CREATE UNIQUE INDEX uniq_invoice_external
ON invoices(user_id, external_id)
WHERE external_id IS NOT NULL;

CREATE TRIGGER trg_invoices_updated_at
BEFORE UPDATE ON invoices
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =========================
-- PAYMENTS
-- =========================
CREATE TABLE payments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    invoice_id    UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    amount        BIGINT NOT NULL CHECK (amount > 0),

    -- Track currency of each payment for multi-currency reconciliation
    currency      CHAR(3) NOT NULL DEFAULT 'THB',
    exchange_rate NUMERIC(20,10),          -- rate to invoice currency at payment time

    method        TEXT NOT NULL DEFAULT 'cash'
                  CHECK (method IN ('cash','transfer','promptpay','other')),

    reference     TEXT,
    proof_url     TEXT,
    note          TEXT,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT payment_currency_check CHECK (char_length(currency) = 3)
);

CREATE INDEX idx_payments_invoice_created ON payments(invoice_id, created_at);
CREATE INDEX idx_payments_user_created    ON payments(user_id, created_at DESC);

-- Idempotency
CREATE UNIQUE INDEX uniq_payment_reference
ON payments(user_id, reference)
WHERE reference IS NOT NULL;

-- =========================
-- MESSAGES
-- =========================
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    invoice_id      UUID REFERENCES invoices(id) ON DELETE CASCADE,
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,

    direction       message_direction NOT NULL,
    channel         message_channel NOT NULL,

    content         TEXT NOT NULL,
    tone            message_tone NOT NULL DEFAULT 'friendly',

    status          message_status NOT NULL DEFAULT 'draft',
    source          message_source NOT NULL DEFAULT 'ai',

    external_message_id TEXT,

    meta            JSONB NOT NULL DEFAULT '{}',

    sent_at         TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    read_at         TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_user_id     ON messages(user_id);
CREATE INDEX idx_messages_customer_id ON messages(customer_id);
CREATE INDEX idx_messages_invoice_id  ON messages(invoice_id);
CREATE INDEX idx_messages_status      ON messages(status);
CREATE INDEX idx_messages_created_at      ON messages(created_at DESC);

-- Partial: inbound messages always have external_message_id; outbound drafts don't
CREATE UNIQUE INDEX uniq_message_external
ON messages(channel, external_message_id)
WHERE external_message_id IS NOT NULL;

-- =========================
-- REMINDERS
-- =========================
CREATE TABLE reminders (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invoice_id       UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    customer_id      UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,

    scheduled_at     TIMESTAMPTZ NOT NULL,

    tone             message_tone NOT NULL DEFAULT 'friendly',

    -- Which channel to send this reminder on (resolved at creation, not at send time)
    channel          message_channel NOT NULL DEFAULT 'line',

    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','processing','sent','failed','cancelled')),

    attempt_number   INT NOT NULL DEFAULT 0,
    max_attempts     INT NOT NULL DEFAULT 5,

    last_error       TEXT,
    next_retry_at    TIMESTAMPTZ,

    external_message_id TEXT,

    sent_at          TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only one active reminder per invoice at a time; preserves full sent/cancelled history
CREATE UNIQUE INDEX uniq_active_reminder
ON reminders(invoice_id)
WHERE status IN ('pending', 'processing');

-- Worker pickup (critical path)
CREATE INDEX idx_reminders_pick
ON reminders(scheduled_at, id)
WHERE status = 'pending';

-- Retry queue
CREATE INDEX idx_reminders_retry
ON reminders(next_retry_at)
WHERE status = 'failed';

CREATE INDEX idx_reminders_user_invoice ON reminders(user_id, invoice_id);
CREATE INDEX idx_reminders_customer     ON reminders(customer_id);

CREATE TRIGGER trg_reminders_updated_at
BEFORE UPDATE ON reminders
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =========================
-- WEBHOOK EVENTS
-- =========================
CREATE TABLE webhook_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider        TEXT NOT NULL
                    CHECK (provider IN ('line','whatsapp','stripe','wechat','telegram')),

    event_id        TEXT NOT NULL,

    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,

    payload         JSONB NOT NULL,
    headers         JSONB,
    signature       TEXT,

    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','processing','processed','failed')),

    attempt_count   INT NOT NULL DEFAULT 0,
    last_error      TEXT,
    next_retry_at   TIMESTAMPTZ,

    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(provider, event_id)
);

CREATE UNIQUE INDEX idx_webhook_unique
ON webhook_events(provider, event_id);

CREATE INDEX idx_webhook_pending
ON webhook_events(received_at)
WHERE status = 'pending';

CREATE INDEX idx_webhook_retry
ON webhook_events(next_retry_at)
WHERE status = 'pending';

CREATE TRIGGER trg_webhook_events_updated_at
BEFORE UPDATE ON webhook_events
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =========================
-- DAILY STATS (user-level)
-- All BIGINT amounts are in the user's home_currency (converted at write time)
-- =========================
CREATE TABLE daily_stats (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date              DATE NOT NULL,

    -- Stored in user's home_currency (see users.home_currency)
    total_outstanding BIGINT NOT NULL DEFAULT 0,
    total_collected   BIGINT NOT NULL DEFAULT 0,

    invoices_unpaid   INT NOT NULL DEFAULT 0,
    invoices_paid     INT NOT NULL DEFAULT 0,

    messages_sent     INT NOT NULL DEFAULT 0,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT unique_user_date UNIQUE (user_id, date),

    CHECK (
        total_outstanding >= 0 AND
        total_collected   >= 0 AND
        invoices_unpaid   >= 0 AND
        invoices_paid     >= 0 AND
        messages_sent     >= 0
    )
);

CREATE INDEX idx_daily_stats_user_date ON daily_stats(user_id, date DESC);
CREATE INDEX idx_daily_stats_date      ON daily_stats(date);

CREATE TRIGGER trg_daily_stats_updated_at
BEFORE UPDATE ON daily_stats
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
