# 05 · Inventory Service

> Stock truth, per SKU, per seller. Reservation model so two buyers can't oversell the last unit.
> Stack: Go · Chi · PostgreSQL · gRPC · Kafka (consumer + producer)

---

## 1. Responsibility

- Track stock per `sku` per `seller_id` (warehouse/location aware).
- **Reserve** stock when an order is placed; **deduct** on payment; **release** on cancel/timeout.
- Consume `suborder.created` → try reserve → emit `inventory.reserved` or `inventory.rejected`.
- Sync warehouse value/levels with CashPilotX (your warehouse-backed stock).

---

## 2. Database schema

```sql
CREATE TABLE stock (
  sku          TEXT NOT NULL,
  seller_id    UUID NOT NULL,
  location     TEXT NOT NULL DEFAULT 'main',
  on_hand      INT  NOT NULL DEFAULT 0,      -- physical units
  reserved     INT  NOT NULL DEFAULT 0,      -- held for unpaid orders
  -- available = on_hand - reserved (never sold below 0)
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (sku, seller_id, location),
  CONSTRAINT stock_non_negative CHECK (on_hand >= 0 AND reserved >= 0 AND reserved <= on_hand)
);

CREATE TYPE reservation_status AS ENUM ('held', 'committed', 'released', 'expired');

CREATE TABLE reservations (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub_order_id  UUID NOT NULL,
  sku           TEXT NOT NULL,
  seller_id     UUID NOT NULL,
  qty           INT  NOT NULL,
  status        reservation_status NOT NULL DEFAULT 'held',
  expires_at    TIMESTAMPTZ NOT NULL,        -- auto-release if unpaid (e.g. +15min)
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reservations_suborder ON reservations(sub_order_id);
CREATE INDEX idx_reservations_expiry   ON reservations(status, expires_at);

-- idempotent consumer dedupe
CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 3. The reservation lifecycle

```
suborder.created  → ReserveStockTx:  reserved += qty (if available)  → inventory.reserved
                                       else                            → inventory.rejected
suborder.paid     → CommitStockTx:    on_hand -= qty; reserved -= qty; reservation=committed
order.cancelled   → ReleaseStockTx:   reserved -= qty; reservation=released
expiry timer      → ReleaseStockTx:   unpaid reservation past expires_at → released
```

Expiry is a periodic sweeper (or RabbitMQ delayed job per reservation) that releases held stock for orders that never paid — prevents inventory being locked forever.

---

## 4. gRPC (internal)

```
inventory.GetStock(sku)              → {available, on_hand}   (gateway BFF "in stock?")
inventory.BatchStock([]sku)          → cart availability check
inventory.AdjustStock(sku, delta)    → seller/admin restock (also via REST)
```

---

## 5. Kafka

| Direction | Topic | Action |
|---|---|---|
| Consume | `suborder.created` | Reserve → emit reserved/rejected |
| Consume | `suborder.paid` | Commit reservation (deduct on_hand) |
| Consume | `order.cancelled` | Release reservation |
| Produce | `inventory.reserved` | `{sub_order_id, items, ok:true}` → payment-service |
| Produce | `inventory.rejected` | `{sub_order_id, sku, reason:"out_of_stock"}` → order, notification |
| Produce | `inventory.snapshot` | compacted current levels → CashPilotX warehouse sync |

---

## 6. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/seller/inventory` | seller | Stock for own SKUs |
| PATCH | `/api/v1/seller/inventory/{sku}` | seller | Restock / adjust |
| GET | `/api/v1/admin/inventory` | admin | Cross-seller view |

---

## 7. Open items

- ⚠ Reservation TTL (15 min default) must match payment-service's charge window — agree in Sprint 2.
- ⚠ Oversell policy for backorder-allowed sellers (negative available?) — default: hard block.
- Concurrency: `ReserveStockTx` uses `SELECT … FOR UPDATE` on the stock row to avoid race on the last unit.
