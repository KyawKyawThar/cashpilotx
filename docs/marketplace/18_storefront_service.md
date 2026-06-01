# 18 · Storefront Service

> Each seller's branded store page — the "shop" you visit on Shopee/Lazada. A read-optimized
> projection that assembles store profile + that seller's products + ratings.
> Stack: Go · Chi · PostgreSQL (read model) · gRPC · Kafka (consumer)

---

## 1. Responsibility

- Render a seller's public store: banner, logo, description, categories, featured products, rating.
- Aggregate the seller's approved products (from product-service via events) + seller rating.
- Activate a store page on `seller.approved`; hide it on `seller.suspended`.
- Pure read model — kept in sync via Kafka, never the write-source for products or sellers.

---

## 2. Database schema (read model)

```sql
CREATE TABLE storefronts (
  seller_id     UUID PRIMARY KEY,
  store_name    TEXT NOT NULL,
  store_slug    TEXT NOT NULL UNIQUE,
  description   TEXT,
  logo_url      TEXT,
  banner_url    TEXT,
  active        BOOLEAN NOT NULL DEFAULT false,  -- flipped by seller.approved/suspended
  rating_avg    NUMERIC(2,1) NOT NULL DEFAULT 0,
  rating_count  INT NOT NULL DEFAULT 0,
  product_count INT NOT NULL DEFAULT 0,
  featured      JSONB NOT NULL DEFAULT '[]',     -- seller-curated product_ids
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- denormalized product cards for fast store browsing (projection of product.updated)
CREATE TABLE storefront_products (
  product_id   UUID PRIMARY KEY,
  seller_id    UUID NOT NULL,
  title        TEXT NOT NULL,
  cover_url    TEXT,
  price_minor  BIGINT NOT NULL,
  rating_avg   NUMERIC(2,1) NOT NULL DEFAULT 0,
  in_stock     BOOLEAN NOT NULL DEFAULT true,
  status       TEXT NOT NULL,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sfproducts_seller ON storefront_products(seller_id, status);

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 3. Kafka (this service is almost pure consumer)

| Direction | Topic | Action |
|---|---|---|
| Consume | `seller.approved` | create/activate storefront row |
| Consume | `seller.suspended` | `active=false` (hide store) |
| Consume | `product.updated` | upsert storefront_products card; recompute product_count |
| Consume | `review.created` (seller target) | update store rating_avg/count |

Because `product.updated` is compacted, a fresh storefront-service rebuilds all store catalogs by replay.

---

## 4. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/stores/{slug}` | any | Store page (profile + featured + rating) |
| GET | `/api/v1/stores/{slug}/products` | any | Browse store products (paginated, filter, sort) |
| PATCH | `/api/v1/seller/store` | seller | Edit banner/description/featured |

---

## 5. Why separate from seller-service

- **seller-service** = write side: onboarding, KYC, balance, status (transactional, private).
- **storefront-service** = read side: fast public browsing, denormalized, rebuildable from events.

Splitting them keeps the public store page fast and isolated from sensitive seller/financial data.

---

## 6. Open items

- ⚠ Store URL strategy (`/stores/{slug}` vs subdomain `{slug}.bazaar.com`).
- ⚠ Featured/curation rules (seller-picked vs best-sellers vs paid promotion).
- ⚠ Store-level search vs global search-service — reuse search-service scoped by seller_id.
