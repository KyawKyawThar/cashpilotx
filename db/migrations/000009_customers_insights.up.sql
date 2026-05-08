CREATE TYPE insight_type_enum AS ENUM (
  'risk',
  'payment_behavior',
  'engagement'
);

-- =========================
-- CUSTOMER INSIGHTS
-- =========================
CREATE TABLE customer_insights (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    customer_id   UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,

    insight_type  insight_type_enum NOT NULL,
    insight_text  TEXT NOT NULL,

    score         NUMERIC(5,2),
    delta_score   NUMERIC(5,2),

    -- Raw evidence used to generate the insight (avg_days_to_pay, invoice counts, etc.)
    -- Needed to explain the insight in the UI and for AI context window
    metadata      JSONB NOT NULL DEFAULT '{}',

    valid_from    TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to      TIMESTAMPTZ,

    is_current    BOOLEAN NOT NULL DEFAULT true,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only one current insight per type per customer
CREATE UNIQUE INDEX uniq_current_insight
ON customer_insights(user_id, customer_id, insight_type)
WHERE is_current = true;

CREATE INDEX idx_customer_insights_lookup
ON customer_insights(user_id, customer_id, insight_type, is_current);