-- best_contact_day / best_contact_time: derived by RefreshRiskProfileTx from
-- customer_events payment patterns. Powers the "Best window open now" banner.
-- best_contact_window: human-readable composite, e.g. "Friday 4–6 pm".
ALTER TABLE customer_risk_profiles
    ADD COLUMN best_contact_day    TEXT,   -- e.g. 'Friday'
    ADD COLUMN best_contact_time   TEXT,   -- e.g. '16:00-18:00'
    ADD COLUMN best_contact_window TEXT;   -- e.g. 'Friday 4–6 pm'
