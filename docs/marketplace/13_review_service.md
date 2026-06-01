# 13 · Review Service

> Product & seller reviews, gated on actual delivery. Star ratings, photos, seller replies, moderation.
> Stack: Go · Chi · PostgreSQL · Kafka (consumer + producer)

---

## 1. Responsibility

- Unlock a review only after `order.delivered` (verified purchase — no fake reviews).
- Capture rating (1–5) + text + photos (via file-service) for **product** and **seller**.
- Let sellers reply; let admins moderate (hide/flag).
- Emit `review.created` → product-service recomputes `rating_avg`; storefront recomputes seller rating.

---

## 2. Database schema

```sql
CREATE TYPE review_target AS ENUM ('product','seller');
CREATE TYPE review_status AS ENUM ('published','hidden','flagged');

-- which (user, product) pairs are allowed to review (set on delivery)
CREATE TABLE reviewable (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       UUID NOT NULL,
  sub_order_id  UUID NOT NULL,
  product_id    UUID NOT NULL,
  seller_id     UUID NOT NULL,
  reviewed      BOOLEAN NOT NULL DEFAULT false,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, sub_order_id, product_id)
);

CREATE TABLE reviews (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  target        review_target NOT NULL,
  target_id     UUID NOT NULL,            -- product_id or seller_id
  user_id       UUID NOT NULL,
  sub_order_id  UUID NOT NULL,
  rating        INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
  body          TEXT,
  photo_urls    JSONB NOT NULL DEFAULT '[]',
  status        review_status NOT NULL DEFAULT 'published',
  seller_reply  TEXT,
  replied_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reviews_target ON reviews(target, target_id, status, created_at DESC);
CREATE UNIQUE INDEX uniq_review_per_purchase ON reviews(user_id, sub_order_id, target, target_id);

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 3. Flow

```
order.delivered  → consume → for each delivered product:
                   INSERT reviewable (user, sub_order, product, seller)
                   → notification: "Rate your purchase"

POST /reviews    → check reviewable row exists & not reviewed
                 → INSERT review (verified purchase) → mark reviewable.reviewed
                 → emit review.created (key=target_id)

review.created   → product-service updates rating_avg/count
                 → storefront-service updates seller rating
```

---

## 4. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/products/{id}/reviews` | any | Paginated, filter by rating |
| GET | `/api/v1/sellers/{id}/reviews` | any | Seller reviews |
| GET | `/api/v1/me/reviewable` | customer | Items awaiting review |
| POST | `/api/v1/reviews` | customer | Submit (verified purchase) · `Idempotency-Key` |
| POST | `/api/v1/seller/reviews/{id}/reply` | seller | Reply to a review |
| POST | `/api/v1/reviews/{id}/report` | any | Report abusive review → flag |
| POST | `/api/v1/admin/reviews/{id}/hide` | admin | Moderate |

---

## 5. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Consume | `order.delivered` | create reviewable rows |
| Produce | `review.created` | key=target_id → product, storefront |
| Produce | `review.flagged` | → notification (moderator alert) |

---

## 6. Open items

- ⚠ One review per product per purchase, or editable within N days? Default: editable 7 days.
- ⚠ Auto-moderation (profanity/spam) before publish, or publish-then-moderate. Default: publish-then-moderate + report.
- ⚠ Should a low seller rating auto-trigger `seller.suspended` review? Out of MVP.
