# 16 · Commission Service

> Splits each paid sub-order into platform fee + seller earnings. The marketplace's revenue engine
> and the feed into CashPilotX's books.
> Stack: Go · PostgreSQL · Kafka (consumer + producer) · outbox

---

## 1. Responsibility

- On `suborder.paid`, compute the **platform fee** and **seller earnings** for that sub-order.
- Apply fee rules: per-seller rate, per-category override, promotions/discounts.
- Maintain an append-only commission ledger (auditable).
- Emit `commission.calculated` → seller-service (balance), payout-service (what to pay), CashPilotX (revenue).
- Reverse on refund.

---

## 2. Database schema

```sql
CREATE TABLE fee_rules (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope         TEXT NOT NULL,            -- 'global' | 'seller' | 'category'
  scope_id      UUID,                     -- seller_id or category_id (null for global)
  rate          NUMERIC(4,3) NOT NULL,    -- 0.080 = 8%
  effective_from TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- append-only ledger; one row per sub_order (idempotent on sub_order_id)
CREATE TABLE commissions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub_order_id    UUID NOT NULL UNIQUE,
  seller_id       UUID NOT NULL,
  gross_minor     BIGINT NOT NULL,        -- buyer paid for this seller's items
  rate            NUMERIC(4,3) NOT NULL,  -- rate applied (snapshot)
  fee_minor       BIGINT NOT NULL,        -- platform revenue
  earnings_minor  BIGINT NOT NULL,        -- seller gets (gross - fee)
  reversed        BOOLEAN NOT NULL DEFAULT false,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_commissions_seller ON commissions(seller_id, created_at DESC);

CREATE TABLE outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  topic TEXT NOT NULL, key TEXT NOT NULL, payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 3. Calculation

```
CalculateCommissionTx (on suborder.paid):
  1. resolve rate: seller-specific rule > category rule > global (8%)
  2. fee_minor      = round(gross_minor * rate)
  3. earnings_minor = gross_minor - fee_minor
  4. INSERT commissions (idempotent on sub_order_id)
  5. outbox: commission.calculated
       { sub_order_id, seller_id, gross, fee, earnings, currency }

ReverseCommissionTx (on payment.refunded):
  mark reversed=true; emit commission.reversed → seller balance + cashpilotx credit note
```

Rounding rule fixed (banker's vs floor) to keep platform fee + earnings = gross exactly.

---

## 4. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Consume | `suborder.paid` | compute split |
| Consume | `payment.refunded` | reverse |
| Produce | `commission.calculated` | key=sub_order_id → seller, payout, **cashpilotx** |
| Produce | `commission.reversed` | → seller, cashpilotx |

---

## 5. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/seller/earnings` | seller | Per-order fee/earnings breakdown |
| GET | `/api/v1/admin/fee-rules` | admin | List rules |
| POST | `/api/v1/admin/fee-rules` | admin | Set seller/category override |
| GET | `/api/v1/admin/revenue` | admin | Platform fee revenue (feeds CashPilotX too) |

---

## 6. Open items

- ⚠ Where do shipping fees and customer discounts sit in the split? (fee on gross-of-shipping or net?)
- ⚠ Tax/VAT handling per market — does the platform withhold? Coordinate with CashPilotX.
- ⚠ Promotions funded by platform vs seller change the earnings math — model before Sprint 4.
