-- Sprint A: Upgrade delivery_services to a proper admin-managed catalog.
-- Adds price, currency, is_active, and created_by for full RBAC-aware management.

ALTER TABLE delivery_services
    ADD COLUMN price      BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN currency   CHAR(3) NOT NULL DEFAULT 'MMK',
    ADD COLUMN is_active  BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN created_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_delivery_services_active ON delivery_services(user_id, is_active);
