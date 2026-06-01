# 04 · Product Service

> The catalog. Products, variants (size/color), categories, photos, and seller ownership.
> Stack: Go · Chi · PostgreSQL · gRPC · Kafka producer · S3/MinIO (via file-service)

---

## 1. Responsibility

- Product catalog: title, description, price, photos, categories, attributes.
- **Variants** (SKU-level): e.g. T-shirt → {Red/S, Red/M, Blue/L}, each its own price + SKU.
- Seller ownership (`seller_id`) and admin approval workflow (multi-vendor).
- Emits `product.updated` so search + storefront stay in sync.
- Does **not** own stock counts — that's inventory-service (keyed by SKU).

---

## 2. Database schema

```sql
CREATE TYPE product_status AS ENUM ('draft', 'pending_review', 'approved', 'rejected', 'archived');

CREATE TABLE categories (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id  UUID REFERENCES categories(id),
  name       TEXT NOT NULL,
  slug       TEXT NOT NULL UNIQUE,
  path       TEXT NOT NULL                -- 'electronics/phones' for breadcrumb/search
);

CREATE TABLE products (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  seller_id    UUID NOT NULL,            -- mirrors seller-service id
  category_id  UUID NOT NULL REFERENCES categories(id),
  title        TEXT NOT NULL,
  description  TEXT NOT NULL,
  brand        TEXT,
  attributes   JSONB NOT NULL DEFAULT '{}',   -- {material, warranty, ...} for filters
  base_price_minor BIGINT NOT NULL,            -- display price; variants may override
  currency     CHAR(3) NOT NULL DEFAULT 'THB',
  status       product_status NOT NULL DEFAULT 'draft',
  reject_reason TEXT,
  rating_avg   NUMERIC(2,1) NOT NULL DEFAULT 0,  -- denormalized from review-service
  rating_count INT NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_products_seller   ON products(seller_id, status);
CREATE INDEX idx_products_category ON products(category_id, status);

CREATE TABLE product_variants (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  sku         TEXT NOT NULL UNIQUE,        -- the unit inventory tracks
  options     JSONB NOT NULL,              -- {color:"Red", size:"M"}
  price_minor BIGINT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_variants_product ON product_variants(product_id);

CREATE TABLE product_photos (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  url         TEXT NOT NULL,               -- CDN url from file-service
  position    INT  NOT NULL DEFAULT 0,
  is_cover    BOOLEAN NOT NULL DEFAULT false
);
```

---

## 3. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/products` | any | List/filter (category, price, brand, rating) |
| GET | `/api/v1/products/{id}` | any | Detail (+ variants, photos) |
| POST | `/api/v1/seller/products` | seller | Create draft · `Idempotency-Key` |
| PATCH | `/api/v1/seller/products/{id}` | seller | Edit (→ back to pending_review) |
| POST | `/api/v1/seller/products/{id}/submit` | seller | Submit for approval |
| POST | `/api/v1/admin/products/{id}/approve` | admin | Approve → `product.updated` |
| POST | `/api/v1/admin/products/{id}/reject` | admin | Reject + reason |
| GET | `/api/v1/categories` | any | Category tree |

---

## 4. gRPC (internal)

```
product.Get(product_id)            → product + variants + photos (gateway BFF, order)
product.GetVariant(sku)            → variant + price + seller_id (cart, order)
product.BatchGet([]product_id)     → for cart/order line rendering
```

---

## 5. Kafka

| Direction | Topic | Payload |
|---|---|---|
| Produce | `product.updated` | `{product_id, seller_id, status, title, price, category, ...}` (compacted topic) |
| Consume | `review.created` | Recompute `rating_avg`, `rating_count` |
| Consume | `seller.suspended` | Set seller's products → `archived` (hide from store) |

`product.updated` is **log-compacted** — search-service and storefront-service can rebuild full current catalog state from the topic.

---

## 6. Transaction functions

```
SubmitProductTx       status: draft → pending_review
ApproveProductTx      status → approved; emit product.updated (outbox)
RejectProductTx       status → rejected + reject_reason; notify seller
ApplyReviewRatingTx   update rating_avg/rating_count from review.created (idempotent on event_id)
```

---

## 7. Open items

- ⚠ Admin approval required for every edit, or only first publish? (anti-abuse vs seller friction)
- ⚠ Attribute schema per category for faceted search — coordinate with search-service.
