# 03 · User Service

> Customer profile & address book. Separate from auth (identity) — this is the "who they are" data.
> Stack: Go · Chi · PostgreSQL · gRPC · Kafka consumer

---

## 1. Responsibility

- Customer profiles (display name, avatar, locale, preferences).
- Delivery address book (multiple addresses, default flag, geocoded lat/lng for delivery).
- Provisioned automatically when `user.registered` arrives.
- Provides addresses to order-service at checkout (via gRPC).

---

## 2. Database schema

```sql
CREATE TABLE profiles (
  user_id      UUID PRIMARY KEY,        -- mirrors auth users.id (no FK across DBs)
  display_name TEXT NOT NULL,
  avatar_url   TEXT,
  locale       TEXT NOT NULL DEFAULT 'th-TH',
  phone        TEXT,
  preferences  JSONB NOT NULL DEFAULT '{}',  -- {notify_email, notify_sms, notify_push}
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE addresses (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID NOT NULL,
  label        TEXT NOT NULL,            -- 'Home' | 'Office'
  recipient    TEXT NOT NULL,
  phone        TEXT NOT NULL,
  line1        TEXT NOT NULL,
  line2        TEXT,
  subdistrict  TEXT, district TEXT, province TEXT,
  postal_code  TEXT,
  country      CHAR(2) NOT NULL DEFAULT 'TH',
  lat          NUMERIC(9,6),            -- for delivery routing/ETA
  lng          NUMERIC(9,6),
  is_default   BOOLEAN NOT NULL DEFAULT false,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_addresses_user ON addresses(user_id);
CREATE UNIQUE INDEX uniq_default_address ON addresses(user_id) WHERE is_default;
```

---

## 3. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/me` | customer | Profile |
| PATCH | `/api/v1/me` | customer | Update profile/preferences |
| GET | `/api/v1/me/addresses` | customer | List addresses |
| POST | `/api/v1/me/addresses` | customer | Add address (geocoded) |
| PATCH | `/api/v1/me/addresses/{id}` | customer | Edit / set default |
| DELETE | `/api/v1/me/addresses/{id}` | customer | Remove |

---

## 4. gRPC (internal)

```
user.GetAddress(address_id)   → full address + lat/lng   (order-service at checkout)
user.GetProfile(user_id)      → display_name, locale, preferences (notification-service)
```

---

## 5. Kafka

| Direction | Topic | Action |
|---|---|---|
| Consume | `user.registered` | Create empty profile row |
| Consume | `order.delivered` | Increment lifetime order count (loyalty stats) |

---

## 6. Open items

- ⚠ Geocoding provider (Google vs Mapbox vs local) — decide with delivery-service in Sprint 3.
- Address `lat/lng` feeds delivery ETA; required for the live map's destination pin.
