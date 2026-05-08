CREATE TABLE customers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Identity
    name            TEXT NOT NULL,
    normalized_name TEXT NOT NULL,

    phone           TEXT,
    normalized_phone TEXT,

    country         CHAR(2),           -- ISO 3166-1 alpha-2: TH, MM, CN (used for auto channel routing)
    -- Contact platform IDs live in customer_contacts (normalized). No line_id/wechat_id here.

    -- Classification
    type            TEXT NOT NULL DEFAULT 'customer', -- customer | supplier | both
    status          TEXT NOT NULL DEFAULT 'active',   -- active | inactive | blocked

    -- Financial snapshot (DENORMALIZED FOR PERFORMANCE — stored in user's home_currency)
    total_owed      BIGINT NOT NULL DEFAULT 0,
    total_paid      BIGINT NOT NULL DEFAULT 0,
    credit_limit    BIGINT NOT NULL DEFAULT 0,

    last_activity   TIMESTAMPTZ,

    -- Metadata
    notes           TEXT,
    tags            TEXT[],

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Constraints
    CONSTRAINT customers_type_check CHECK (type IN ('customer', 'supplier', 'both')),
    CONSTRAINT customers_status_check CHECK (status IN ('active', 'inactive', 'blocked')),
    CONSTRAINT customers_amount_check CHECK (total_owed >= 0 AND total_paid >= 0)
);



CREATE TYPE contact_platform AS ENUM (
  'LINE',
  'WECHAT',
  'VIBER',
  'MESSENGER',
  'TELEGRAM',
  'WHATSAPP'
);


CREATE TABLE customer_contacts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    customer_id   UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,

    platform      contact_platform NOT NULL,

    external_id   TEXT NOT NULL,
    display_name  TEXT,

    meta          JSONB NOT NULL DEFAULT '{}',

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    is_active BOOLEAN NOT NULL DEFAULT true
);

-- fast lookup by customer
CREATE INDEX idx_contacts_customer
ON customer_contacts(customer_id);

-- fast lookup by platform identity (VERY important)
CREATE INDEX idx_contacts_platform_lookup
ON customer_contacts(user_id, platform, external_id);


CREATE UNIQUE INDEX uniq_active_contact
ON customer_contacts(user_id, platform, external_id)
WHERE is_active = true;

CREATE TRIGGER trg_customers_updated_at
BEFORE UPDATE ON customers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();