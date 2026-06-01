# 15 · Seller Service

> The multi-vendor core. Seller onboarding, KYC, store profile, bank details, status, and balance.
> This is what makes it a marketplace (invite distributors) instead of single-vendor e-commerce.
> Stack: Go · Chi · PostgreSQL · gRPC · Kafka (producer + consumer) · file-service (KYC)

---

## 1. Responsibility

- Seller registration & onboarding wizard (store info → KYC docs → bank account → submit).
- KYC document collection (private files via file-service) and admin approval.
- Store profile (name, logo, description, policies) — surfaced by storefront-service.
- Track seller **balance** (pending vs available) updated by commission + payout events.
- Status lifecycle: `pending → approved → active → suspended`.

---

## 2. Database schema

```sql
CREATE TYPE seller_status AS ENUM ('pending','approved','active','suspended','rejected');

CREATE TABLE sellers (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       UUID NOT NULL UNIQUE,     -- the auth user with role='seller'
  store_name    TEXT NOT NULL,
  store_slug    TEXT NOT NULL UNIQUE,
  description   TEXT,
  logo_url      TEXT,
  status        seller_status NOT NULL DEFAULT 'pending',
  reject_reason TEXT,
  commission_rate NUMERIC(4,3) NOT NULL DEFAULT 0.080,  -- 8% default; admin can override per seller
  rating_avg    NUMERIC(2,1) NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE kyc_documents (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  seller_id   UUID NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
  doc_type    TEXT NOT NULL,              -- 'national_id' | 'business_reg' | 'tax_id'
  file_id     UUID NOT NULL,              -- file-service (private)
  verified    BOOLEAN NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bank_accounts (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  seller_id    UUID NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
  bank_name    TEXT NOT NULL,
  account_name TEXT NOT NULL,
  account_no   TEXT NOT NULL,             -- encrypted at rest
  is_default   BOOLEAN NOT NULL DEFAULT true
);

-- running balance (source of truth for "how much do we owe this seller")
CREATE TABLE seller_balances (
  seller_id      UUID PRIMARY KEY REFERENCES sellers(id),
  pending_minor  BIGINT NOT NULL DEFAULT 0,  -- earned, in hold period
  available_minor BIGINT NOT NULL DEFAULT 0, -- released, payable
  paid_out_minor BIGINT NOT NULL DEFAULT 0,  -- lifetime
  currency       CHAR(3) NOT NULL DEFAULT 'THB',
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 3. Onboarding flow

```
1. user (role=seller) POST /seller/onboard {store_name, description}  → seller 'pending'
                                                                       → seller.registered
2. upload KYC docs (file-service, private) + bank account
3. submit for review
4. admin POST /admin/sellers/{id}/approve → status 'approved' → seller.approved
                                          → storefront-service activates store page
                                          → product-service allows listing
5. seller lists products (product-service, pending_review each)
```

---

## 4. Balance updates (from events)

```
commission.calculated → pending_minor += seller_earnings   (still in hold period)
payout.processed      → pending_minor -= amount; paid_out_minor += amount
order.delivered + hold elapsed (signaled by payout-service) → pending → available
payment.refunded      → reverse the relevant earnings
```

---

## 5. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/seller/onboard` | seller | Start onboarding → `seller.registered` |
| POST | `/api/v1/seller/kyc` | seller | Upload KYC doc (file_id) |
| POST | `/api/v1/seller/bank` | seller | Add bank account |
| POST | `/api/v1/seller/submit` | seller | Submit for approval |
| GET | `/api/v1/seller/dashboard` | seller | Balance, orders, ratings |
| GET | `/api/v1/admin/sellers` | admin | Pending/active sellers |
| POST | `/api/v1/admin/sellers/{id}/approve` | admin | → `seller.approved` |
| POST | `/api/v1/admin/sellers/{id}/suspend` | admin | → `seller.suspended` |

---

## 6. gRPC (internal)

```
seller.Get(seller_id)          → store_name, status, commission_rate (order, commission, storefront)
seller.GetCommissionRate(id)   → rate (commission-service)
```

---

## 7. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Produce | `seller.registered` | → notification (onboarding) |
| Produce | `seller.approved` | → storefront, product, notification |
| Produce | `seller.suspended` | → product (archive), storefront, notification |
| Consume | `commission.calculated` | balance pending += earnings |
| Consume | `payout.processed` | balance pending→paid_out |
| Consume | `payment.refunded` | reverse earnings |

---

## 8. Open items

- ⚠ KYC verification: manual admin review (MVP) vs automated identity provider.
- ⚠ Per-seller commission overrides (negotiated rates) vs flat platform rate.
- ⚠ `account_no` encryption — use the same encryption-key approach as CashPilotX config.
