CREATE TABLE delivery_services (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT delivery_services_user_name_unique UNIQUE (user_id, name)
);

CREATE INDEX idx_delivery_services_user ON delivery_services(user_id);

ALTER TABLE invoices ADD COLUMN delivery_service TEXT;
