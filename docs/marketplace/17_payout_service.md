# 17 · Payout Service

> Pays sellers their earnings after a hold period (return window). Batches transfers and
> records them. Classic **RabbitMQ delayed-job** territory on top of Kafka events.
> Stack: Go · PostgreSQL · Kafka (consumer + producer) · RabbitMQ (delayed + transfer jobs) · outbox

---

## 1. Responsibility

- Start a **hold timer** when an item is delivered (3–7 days for returns/disputes).
- After hold elapses, move earnings from `pending` → `available` and create a payout.
- Batch payouts per seller (e.g. weekly) and execute bank transfers.
- Emit `payout.processed` → seller-service (balance), notification, CashPilotX (outgoing payment).

---

## 2. Why RabbitMQ delayed jobs

The hold period is "do this thing N days later, exactly once" — that's a RabbitMQ delayed message,
not a Kafka event:

```
order.delivered (Kafka) → schedule RabbitMQ delayed job (delay = hold_days)
                          → when it fires: ReleaseEarningsTx (pending → available)
weekly cron / batch trigger → RabbitMQ transfer job per seller → bank transfer → retry/DLQ
```

---

## 3. Database schema

```sql
CREATE TYPE payout_status AS ENUM ('scheduled','processing','paid','failed');

-- earnings awaiting release (one per delivered sub_order)
CREATE TABLE holds (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub_order_id  UUID NOT NULL UNIQUE,
  seller_id     UUID NOT NULL,
  earnings_minor BIGINT NOT NULL,
  released      BOOLEAN NOT NULL DEFAULT false,
  release_at    TIMESTAMPTZ NOT NULL,      -- delivered_at + hold_days
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_holds_release ON holds(released, release_at);

CREATE TABLE payouts (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  seller_id     UUID NOT NULL,
  amount_minor  BIGINT NOT NULL,
  currency      CHAR(3) NOT NULL DEFAULT 'THB',
  bank_account_id UUID NOT NULL,
  status        payout_status NOT NULL DEFAULT 'scheduled',
  provider_ref  TEXT,
  period_start  DATE, period_end DATE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payouts_seller ON payouts(seller_id, created_at DESC);

CREATE TABLE payout_items (              -- which holds make up a payout
  payout_id UUID NOT NULL REFERENCES payouts(id) ON DELETE CASCADE,
  hold_id   UUID NOT NULL REFERENCES holds(id),
  PRIMARY KEY (payout_id, hold_id)
);

CREATE TABLE outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  topic TEXT NOT NULL, key TEXT NOT NULL, payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 4. Flow

```
commission.calculated → INSERT hold (released=false, release_at = now + hold_days placeholder)
order.delivered       → set hold.release_at = delivered_at + hold_days
                        → schedule RabbitMQ delayed job

(delay fires) ReleaseEarningsTx → hold.released = true
                                → seller balance pending → available (via seller-service consuming events,
                                  or direct signal)

weekly batch → BatchPayoutTx per seller:
   sum released-but-unpaid holds → INSERT payout (scheduled) + payout_items
   → RabbitMQ transfer job → bank transfer (retry → DLQ)
   → on success: payout.status=paid → outbox payout.processed
```

---

## 5. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Consume | `commission.calculated` | create hold |
| Consume | `order.delivered` | start hold timer (delayed job) |
| Consume | `payment.refunded` | cancel/claw back hold if not yet paid |
| Produce | `payout.processed` | key=payout_id → seller, notification, **cashpilotx** |

---

## 6. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/seller/payouts` | seller | Payout history + upcoming |
| GET | `/api/v1/seller/payouts/{id}` | seller | Detail (which orders) |
| GET | `/api/v1/admin/payouts` | admin | All payouts, retry failed |
| POST | `/api/v1/admin/payouts/run` | admin | Trigger batch manually |

---

## 7. Open items

- ⚠ Hold period length (3 vs 7 days) and whether disputes extend it.
- ⚠ Payout schedule (weekly vs on-demand vs threshold) and minimum payout amount.
- ⚠ Transfer rail per market (bank API, PromptPay, KBZPay disbursement) — ties to master ⚠ GAP 2.
- ⚠ Refund after payout already sent → negative balance / clawback policy.
