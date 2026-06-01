# 08 · Payment Service

> Collects money once, knows it belongs to many sellers, and handles refunds.
> Stack: Go · Chi · PostgreSQL · Kafka (consumer + producer) · outbox · provider SDKs (Stripe/KBZPay/Wave)

---

## 1. Responsibility

- Charge the customer **once** for a multi-seller order (after all stock reserved).
- Record the payment + the per-sub-order split breakdown.
- Emit `order.paid` (parent) + `suborder.paid` (per seller) for downstream split/ship/payout.
- Handle refunds (full + partial) on cancellation/return.
- Validate payment-provider webhooks.

> The actual seller payout happens later in **payout-service**; payment-service only collects + records intent to split. See master **⚠ GAP 2** (charge-then-split vs connected accounts).

---

## 2. Database schema

```sql
CREATE TYPE payment_status AS ENUM ('pending', 'authorized', 'captured', 'failed', 'refunded', 'partially_refunded');

CREATE TABLE payments (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id      UUID NOT NULL,            -- parent order
  user_id       UUID NOT NULL,
  provider      TEXT NOT NULL,            -- 'stripe' | 'kbzpay' | 'wave' | 'cod'
  provider_ref  TEXT,                     -- charge id from provider
  amount_minor  BIGINT NOT NULL,
  currency      CHAR(3) NOT NULL DEFAULT 'THB',
  status        payment_status NOT NULL DEFAULT 'pending',
  idempotency_key TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_order ON payments(order_id);

-- the per-seller split of one payment
CREATE TABLE payment_splits (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id    UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
  sub_order_id  UUID NOT NULL,
  seller_id     UUID NOT NULL,
  gross_minor   BIGINT NOT NULL,          -- what the buyer paid for this seller's items
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- (commission/fee is computed in commission-service, not here)

CREATE TABLE refunds (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id    UUID NOT NULL REFERENCES payments(id),
  sub_order_id  UUID,                     -- null = full-order refund
  amount_minor  BIGINT NOT NULL,
  reason        TEXT,
  provider_ref  TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  topic TEXT NOT NULL, key TEXT NOT NULL, payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 3. Charge flow

```
order-service: all sub_orders reserved → calls payment.Charge(order_id) [gRPC]
  OR  payment-service consumes a "ready_to_charge" signal (per ⚠ GAP 1 orchestration choice)

ChargePaymentTx:
  1. Call provider (Stripe/KBZPay/Wave) — idempotency_key prevents double charge
  2. On capture:
     INSERT payments (captured)
     INSERT payment_splits per sub_order (gross only)
     outbox: order.paid (parent)  +  suborder.paid ×N
  3. On failure:
     INSERT payments (failed) → emit payment.failed → order-service compensates (release stock)
```

Outbox guarantees that a captured charge always results in an `order.paid` event even if the
process crashes right after the provider returns.

---

## 4. Refund flow

```
order.cancelled / return approved → RefundTx:
  1. Call provider refund (full or per sub_order amount)
  2. INSERT refunds; update payment status → refunded / partially_refunded
  3. emit payment.refunded → commission-service (reverse fee), cashpilotx (credit note)
```

---

## 5. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/payments/intent` | customer | Create provider payment intent for an order |
| POST | `/api/v1/payments/confirm` | customer | Confirm/capture · `Idempotency-Key` |
| GET | `/api/v1/payments/{id}` | customer | Status |
| POST | `/webhooks/stripe` | provider sig | Capture/refund confirmations → reconcile |
| POST | `/webhooks/kbzpay` | provider sig | SE Asia rail webhook |

Webhook handlers validate signature → enqueue reconcile job → return 200 fast.

---

## 6. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Produce | `order.paid` | key=parent_order_id → inventory, delivery, notification, **cashpilotx** |
| Produce | `suborder.paid` | key=sub_order_id → commission, seller, delivery |
| Produce | `payment.failed` | → order-service compensation |
| Produce | `payment.refunded` | → commission, cashpilotx |
| Consume | `inventory.reserved` | (if orchestrating charge on full reservation) |
| Consume | `order.cancelled` | trigger refund |

---

## 7. Open items

- ⚠ Resolve master **GAP 2** (split model) — this doc assumes charge-to-platform + later payout.
- ⚠ COD (cash on delivery) is huge in SE Asia: `provider='cod'` means no upfront capture; `order.paid` is emitted only when delivery-service confirms cash collected. Model COD before launch.
- ⚠ Provider selection per market (Stripe intl, KBZPay/Wave for Myanmar, PromptPay for Thailand).
