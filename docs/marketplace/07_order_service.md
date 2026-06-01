# 07 · Order Service

> The heart of the platform. Parent order + per-seller sub-orders, a state machine, and the
> **saga orchestrator** for the money-critical path (reserve → pay → ship → deliver).
> Stack: Go · Chi · PostgreSQL · gRPC · Kafka (producer + consumer) · SSE · outbox pattern

---

## 1. Responsibility

- Turn a checkout snapshot into a **parent order** + N **sub-orders** (one per seller).
- Own the order **state machine** and orchestrate the distributed transaction (see master ⚠ GAP 1 — orchestration chosen for the money path).
- Emit lifecycle events: `order.created`, `suborder.created`, `order.delivered`, `order.cancelled`.
- Aggregate sub-order statuses into a parent status the customer sees.
- Stream live order status to the client via SSE.

---

## 2. Why parent + sub-orders

A cart from 3 sellers = 1 payment, 3 fulfillments, 3 parcels, 3 trackings, 3 payouts.

```
parent_order (ORD-2026-0001)         ← one charge, one customer-facing order
├── sub_order A  (seller A)          ← own state, own tracking, own payout
├── sub_order B  (seller B)
└── sub_order C  (seller C)
```

---

## 3. Database schema

```sql
CREATE TYPE order_status AS ENUM (
  'pending_payment', 'paid', 'partially_shipped', 'shipped',
  'partially_delivered', 'delivered', 'cancelled', 'refunded'
);

CREATE TYPE suborder_status AS ENUM (
  'reserving', 'reserved', 'rejected', 'paid',
  'preparing', 'shipped', 'delivered', 'cancelled', 'refunded'
);

CREATE TABLE orders (                     -- parent
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  external_id   TEXT NOT NULL UNIQUE,     -- ORD-2026-0001 (generated server-side)
  user_id       UUID NOT NULL,
  address_id    UUID NOT NULL,            -- snapshot of delivery address below
  ship_to       JSONB NOT NULL,           -- frozen address copy
  status        order_status NOT NULL DEFAULT 'pending_payment',
  subtotal_minor BIGINT NOT NULL,
  shipping_minor BIGINT NOT NULL DEFAULT 0,
  total_minor   BIGINT NOT NULL,
  currency      CHAR(3) NOT NULL DEFAULT 'THB',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_orders_user ON orders(user_id, created_at DESC);

CREATE TABLE sub_orders (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  seller_id     UUID NOT NULL,
  status        suborder_status NOT NULL DEFAULT 'reserving',
  subtotal_minor BIGINT NOT NULL,
  tracking_no   TEXT,                      -- set by delivery-service later
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_suborders_order  ON sub_orders(order_id);
CREATE INDEX idx_suborders_seller ON sub_orders(seller_id, status);

CREATE TABLE order_items (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub_order_id UUID NOT NULL REFERENCES sub_orders(id) ON DELETE CASCADE,
  sku          TEXT NOT NULL,
  product_id   UUID NOT NULL,
  title        TEXT NOT NULL,             -- frozen at order time
  qty          INT NOT NULL,
  unit_price_minor BIGINT NOT NULL,
  line_total_minor BIGINT NOT NULL
);

-- transactional outbox (events published reliably after commit)
CREATE TABLE outbox (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  topic       TEXT NOT NULL,
  key         TEXT NOT NULL,
  payload     JSONB NOT NULL,
  published   BOOLEAN NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbox_unpub ON outbox(published, created_at) WHERE NOT published;

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ DEFAULT now());
```

---

## 4. State machine

```
PARENT                                  SUB-ORDER
pending_payment                         reserving ─→ reserved ─→ paid
   │  (all subs paid)                                    │  rejected → (cancel whole order)
   ▼                                                     ▼
 paid ─────────────────────────────────► preparing ─→ shipped ─→ delivered
   │                                                     │
   │  (some subs shipped)                                │ cancel → cancelled → refunded
   ▼
partially_shipped → shipped → partially_delivered → delivered
```

Parent status is **derived** from its sub-orders (e.g. all delivered → `delivered`; some → `partially_delivered`).

---

## 5. Saga orchestration (the money path)

```
CreateOrderTx (from checkout snapshot)
  1. INSERT parent order (pending_payment) + sub_orders (reserving) + items
  2. outbox: order.created  +  suborder.created ×N
  3. commit  → outbox relay publishes to Kafka

consume inventory.reserved   → mark sub_order 'reserved'; when ALL reserved → tell payment to charge
consume inventory.rejected   → CompensateTx: release others, cancel order, refund if charged
consume order.paid           → mark subs 'paid'; emit nothing new (payment already emitted)
consume delivery.status.changed(delivered)
                             → mark sub_order 'delivered'; if all → parent 'delivered'
                             → outbox: order.delivered (per sub_order)

Compensation (any step fails): release reservations + refund + order.cancelled.
```

> This explicit orchestration (vs pure choreography) is the decision in master **⚠ GAP 1** — chosen so
> "where is order X stuck?" is answerable from one state machine, and compensation is centralized.

---

## 6. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/orders` | customer | Create from checkout snapshot · `Idempotency-Key` |
| GET | `/api/v1/orders` | customer | Order history |
| GET | `/api/v1/orders/{id}` | customer | Detail (parent + sub-orders + items + tracking) |
| GET | `/api/v1/orders/{id}/stream` | customer | **SSE** live status updates |
| POST | `/api/v1/orders/{id}/cancel` | customer | Cancel (if not yet shipped) |
| GET | `/api/v1/seller/orders` | seller | Seller's sub-orders queue |
| POST | `/api/v1/seller/suborders/{id}/ship` | seller | Mark ready → triggers delivery shipment |

---

## 7. gRPC (internal)

```
order.GetSubOrder(sub_order_id)   → seller_id, items, ship_to  (delivery, commission)
order.GetOrder(order_id)          → full order (payment, gateway BFF)
```

---

## 8. Kafka

| Direction | Topic | Notes |
|---|---|---|
| Produce | `order.created` | key=parent_order_id |
| Produce | `suborder.created` | key=sub_order_id (one per seller) |
| Produce | `order.delivered` | key=sub_order_id |
| Produce | `order.cancelled` | key=parent_order_id |
| Consume | `inventory.reserved` / `inventory.rejected` | saga step |
| Consume | `order.paid` | mark paid |
| Consume | `delivery.status.changed` | advance sub_order status |

All produces go through the **outbox** table (write + publish atomic-ish).

---

## 9. Open items

- ⚠ Resolve master **GAP 1** (orchestration vs choreography) — this doc assumes orchestration here.
- ⚠ Partial cancellation: can a customer cancel ONE sub-order while others ship? Default: yes, per sub-order.
- ⚠ Shipping fee calculation per seller (flat vs weight vs distance) — needs delivery-service input.
