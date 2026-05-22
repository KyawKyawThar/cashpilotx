-- Sprint A: Product catalog. cost_price is never exposed to team_leader/team_member
-- (stripped at service layer in Sprint D — stored here for admin/super_admin use).

CREATE TYPE product_origin AS ENUM ('thai', 'chinese', 'other');

CREATE TABLE products (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_owner_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    sku           TEXT,
    origin        product_origin NOT NULL DEFAULT 'other',
    category      TEXT,
    unit          TEXT,
    cost_price    BIGINT NOT NULL DEFAULT 0,
    selling_price BIGINT NOT NULL DEFAULT 0,
    currency      CHAR(3) NOT NULL DEFAULT 'MMK',
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT products_org_sku_unique UNIQUE (org_owner_id, sku)
);

CREATE INDEX idx_products_org_owner    ON products(org_owner_id);
CREATE INDEX idx_products_org_active   ON products(org_owner_id, is_active);
