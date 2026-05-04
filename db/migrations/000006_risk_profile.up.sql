-- stable domains only
CREATE TYPE late_fee_type_enum AS ENUM ('flat','percentage');

CREATE TABLE customer_risk_profiles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,

    total_invoices      INT NOT NULL DEFAULT 0,

    profile_confidence  TEXT NOT NULL
        CHECK (profile_confidence IN ('new','learning','confident')),

    risk_score          INT NOT NULL CHECK (risk_score BETWEEN 0 AND 100),

    risk_level          TEXT NOT NULL
        CHECK (risk_level IN ('excellent','good','moderate','risky','bad')),

    avg_days_to_pay     NUMERIC(10,2),

    -- Invoice counts (how many invoices fell into each bucket)
    invoices_on_time    INT NOT NULL DEFAULT 0,
    invoices_late       INT NOT NULL DEFAULT 0,
    invoices_unpaid     INT NOT NULL DEFAULT 0,

    -- Amount totals in minor units, stored in the user's home_currency
    amount_on_time      BIGINT NOT NULL DEFAULT 0,
    amount_late         BIGINT NOT NULL DEFAULT 0,
    amount_unpaid       BIGINT NOT NULL DEFAULT 0,

    longest_overdue     INT NOT NULL DEFAULT 0,   -- days

    late_fee_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    late_fee_pct        NUMERIC(5,2) NOT NULL DEFAULT 0,
    late_fee_type       late_fee_type_enum NOT NULL DEFAULT 'flat',
    total_late_fees     NUMERIC(14,2) NOT NULL DEFAULT 0,

    last_calculated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(user_id, customer_id),

    -- Invoice counts must not exceed total_invoices
    CHECK (invoices_on_time + invoices_late + invoices_unpaid <= total_invoices),

    -- Late fee requires a non-zero percentage
    CHECK (
        (late_fee_enabled = FALSE) OR
        (late_fee_enabled = TRUE AND late_fee_pct > 0)
    )
);

CREATE INDEX idx_customer_risk_user_customer
ON customer_risk_profiles(user_id, customer_id);

CREATE INDEX idx_customer_risk_score
ON customer_risk_profiles(user_id, risk_score);

CREATE INDEX idx_customer_risk_level
ON customer_risk_profiles(user_id, risk_level);

CREATE TRIGGER trg_risk_profiles_updated_at
BEFORE UPDATE ON customer_risk_profiles
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
