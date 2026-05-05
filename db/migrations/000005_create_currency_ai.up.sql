-- ENUM
CREATE TYPE rate_source AS ENUM ('manual', 'api', 'system');

-- EXTENSION (required for exclusion constraint)
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- TABLE
CREATE TABLE exchange_rates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    from_currency   TEXT NOT NULL,
    to_currency     TEXT NOT NULL,

    rate            NUMERIC(20,10) NOT NULL CHECK (rate > 0),

    source          rate_source NOT NULL DEFAULT 'manual',

    valid_from      TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to        TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- ✅ time correctness
    CONSTRAINT chk_valid_time
    CHECK (valid_to IS NULL OR valid_to > valid_from),

    -- ✅ currency correctness
    CONSTRAINT chk_currency_format
    CHECK (
      from_currency ~ '^[A-Z]{3}$'
      AND to_currency ~ '^[A-Z]{3}$'
    ),

    -- ✅ prevent nonsense pairs
    CONSTRAINT chk_different_currencies
    CHECK (from_currency <> to_currency)
);

-- ✅ CRITICAL: no overlapping time ranges
ALTER TABLE exchange_rates
ADD CONSTRAINT no_overlap
EXCLUDE USING gist (
  user_id WITH =,
  from_currency WITH =,
  to_currency WITH =,
  tstzrange(valid_from, COALESCE(valid_to, 'infinity')) WITH &&
);

-- ✅ latest lookup (replaces is_active)
CREATE INDEX idx_exchange_rates_active
ON exchange_rates(user_id, from_currency, to_currency)
WHERE valid_to IS NULL;

-- ✅ historical queries
CREATE INDEX idx_exchange_rates_time
ON exchange_rates(user_id, from_currency, to_currency, valid_from DESC);


CREATE TRIGGER trg_exchange_rates_updated_at
BEFORE UPDATE ON exchange_rates
FOR EACH ROW EXECUTE FUNCTION set_updated_at();