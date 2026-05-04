CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone           TEXT NOT NULL,
    pin_hash        TEXT NOT NULL,
    name            TEXT NOT NULL,
    business_name   TEXT,
    locale          TEXT DEFAULT 'th',
    plan            TEXT NOT NULL DEFAULT 'free',
    home_currency   TEXT NOT NULL DEFAULT 'THB',

    streak_days     INT NOT NULL DEFAULT 0,
    streak_best     INT NOT NULL DEFAULT 0,

    -- Onboarding (Phase 1): 0=new 1=currency set 2=channel set 3=complete
    -- (Step 2 was "ai_key set" / BYOK — deferred to Phase 2)
    onboarding_step     SMALLINT NOT NULL DEFAULT 0,
    onboarding_done_at  TIMESTAMPTZ,

    is_active       BOOLEAN NOT NULL DEFAULT true,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_phone_format CHECK (phone ~ '^\+?[0-9]{8,15}$'),
    CONSTRAINT users_plan_check CHECK (plan IN ('free', 'pro', 'enterprise')),
    CONSTRAINT users_currency_check CHECK (char_length(home_currency) = 3),
    CONSTRAINT users_streak_check CHECK (streak_best >= streak_days),
    CONSTRAINT users_onboarding_step_check CHECK (onboarding_step BETWEEN 0 AND 4)
);

-- Unique only for active users (allows reuse after deactivation)
CREATE UNIQUE INDEX uniq_active_phone
ON users(phone)
WHERE is_active = true;

-- Fast lookup indexes
CREATE INDEX idx_users_active_id
ON users(id)
WHERE is_active = true;

CREATE INDEX idx_users_active_phone
ON users(phone)
WHERE is_active = true;

-- =========================
-- SESSIONS TABLE
-- =========================
CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token   TEXT NOT NULL,
    user_agent      TEXT,
    ip_address      TEXT,
    device_id       TEXT,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique token
CREATE UNIQUE INDEX idx_sessions_refresh_token
ON sessions(refresh_token);

-- Optimized lookup for auth flow
CREATE INDEX idx_sessions_token_valid
ON sessions(refresh_token, expires_at);

-- Cleanup queries
CREATE INDEX idx_sessions_expires_at
ON sessions(expires_at);

-- =========================
-- UPDATED_AT TRIGGER
-- =========================
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = clock_timestamp();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();