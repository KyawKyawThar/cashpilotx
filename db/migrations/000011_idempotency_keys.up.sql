CREATE TABLE idempotency_keys (
  key        TEXT        PRIMARY KEY,           -- UUID sent by client as Idempotency-Key header
  user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  response   JSONB       NOT NULL,              -- cached response body
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_idempotency_created ON idempotency_keys(created_at);
-- Cleanup job: DELETE FROM idempotency_keys WHERE created_at < now() - interval '24 hours'

