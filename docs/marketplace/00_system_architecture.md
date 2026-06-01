# CashPilotX Marketplace — System Architecture (Master)

> Codename: `bazaar` — multi-vendor e-commerce marketplace (Shopee/Lazada-style) for SE Asia
> Backend: Go · PostgreSQL (per service) · Kafka · RabbitMQ · Redis · gRPC · WebSocket/SSE
> Frontend: React 18 + Vite PWA · TanStack Query · Mapbox/Google Maps (delivery map)
> Companion system: **CashPilotX** (admin/financial OS) — receives `order.paid`, `commission.calculated`, `payout.processed`

> **Purpose of this doc:** the whole-platform view. Per-service detail lives in `01_…`–`18_…`.
> Cross-service open decisions are tracked here as `⚠ GAP` and must be resolved before the sprint they affect.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Service Catalog](#2-service-catalog)
3. [Technology Decisions](#3-technology-decisions)
4. [Kafka Topology — The Event Backbone](#4-kafka-topology--the-event-backbone)
5. [RabbitMQ vs Kafka vs Redis — When to Use What](#5-rabbitmq-vs-kafka-vs-redis--when-to-use-what)
6. [The Multi-Seller Order Flow (end to end)](#6-the-multi-seller-order-flow-end-to-end)
7. [Money Flow & CashPilotX Bridge](#7-money-flow--cashpilotx-bridge)
8. [Cross-Cutting Concerns](#8-cross-cutting-concerns)
9. [Repository Layout](#9-repository-layout)
10. [Sprint Plan](#10-sprint-plan)
11. [Open Decisions](#11-open-decisions)

---

## 1. System Overview

### Architecture diagram

```
┌───────────────────────────────────────────────────────────────────────┐
│                      CLIENTS (Web PWA · Mobile · Seller Center)         │
│   React 18 + Vite · TanStack Query · Mapbox (live delivery map)         │
└──────────────────────────────┬────────────────────────────────────────┘
                               │ HTTPS (REST) + WSS + SSE
┌──────────────────────────────▼────────────────────────────────────────┐
│                          1 · API GATEWAY                                │
│   Chi router · PASETO verify · rate limit (Redis) · request routing     │
│   BFF aggregation · WS/SSE proxy · CORS · request-id propagation        │
└───┬───────┬───────┬───────┬───────┬───────┬───────┬───────┬─────────────┘
    │ gRPC / REST (internal)                                              │
┌───▼──┐ ┌──▼───┐ ┌─▼────┐ ┌▼─────┐ ┌▼─────┐ ┌▼─────┐ ┌▼──────┐ ┌▼────────┐
│ 2    │ │ 3    │ │ 4    │ │ 5    │ │ 6    │ │ 7    │ │ 8     │ │ 9       │
│ auth │ │ user │ │ prod │ │ inv  │ │ cart │ │order │ │ pay   │ │delivery │
└──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──┬───┘ └───┬───┘ └────┬────┘
                                                │         │          │
                              ┌─────────────────▼─────────▼──────────▼──────┐
                              │                  KAFKA                       │
                              │  order.* · payment.* · delivery.* · review.* │
                              │  seller.* · inventory.* · commission.* …     │
                              └───┬───────┬───────┬───────┬───────┬──────────┘
                                  │       │       │       │       │
                          ┌───────▼─┐ ┌───▼───┐ ┌─▼────┐ ┌▼─────┐ ┌▼──────────┐
                          │ 10      │ │ 11    │ │ 13   │ │ 16   │ │ 17        │
                          │ notify  │ │search │ │review│ │commis│ │ payout    │
                          └────┬────┘ └───────┘ └──────┘ └──┬───┘ └────┬──────┘
                               │ RabbitMQ (send jobs)        │          │
                               ▼                             ▼          ▼
                         email/SMS/push                  CashPilotX (admin OS)

  Other services:  12 file-service (S3/MinIO)   14 chat-service (WS)
                   15 seller-service             18 storefront-service
```

### Service runtime model

Each numbered service is **its own deployable** (own repo dir, own `go.mod` workspace member, own
PostgreSQL database, own Docker image, own K8s Deployment). Services **never share a database**.
They talk via:

- **gRPC** — synchronous reads/commands needed *right now* (e.g. order-service asks inventory-service "is this in stock?").
- **Kafka** — asynchronous facts that *already happened* (e.g. `order.paid`). Fire-and-forget, many consumers, replayable.
- **RabbitMQ** — one-time work that must be *done exactly once* with retry/delay (e.g. "send this email", "run this payout").

---

## 2. Service Catalog

| # | Service | Owns (data) | Talks via | Doc |
|---|---|---|---|---|
| 1 | **api-gateway** | nothing (stateless) + Redis rate-limit | REST in, gRPC/REST out | [01](01_api_gateway.md) |
| 2 | **auth-service** | users, credentials, refresh tokens | gRPC, Kafka (prod) | [02](02_auth_service.md) |
| 3 | **user-service** | profiles, addresses | gRPC, Kafka (cons) | [03](03_user_service.md) |
| 4 | **product-service** | products, variants, categories | gRPC, Kafka (prod) | [04](04_product_service.md) |
| 5 | **inventory-service** | stock, reservations | gRPC, Kafka (both) | [05](05_inventory_service.md) |
| 6 | **cart-service** | carts (Redis) | gRPC | [06](06_cart_service.md) |
| 7 | **order-service** | parent orders, sub-orders, items | gRPC, Kafka (both) | [07](07_order_service.md) |
| 8 | **payment-service** | payments, refunds, splits | gRPC, Kafka (both) | [08](08_payment_service.md) |
| 9 | **delivery-service** | shipments, checkpoints, GPS points | Kafka (both), WS | [09](09_delivery_service.md) |
| 10 | **notification-service** | notification log, templates | Kafka (cons), RabbitMQ | [10](10_notification_service.md) |
| 11 | **search-service** | Elasticsearch index | Kafka (cons) | [11](11_search_service.md) |
| 12 | **file-service** | file metadata (S3/MinIO objects) | REST, RabbitMQ | [12](12_file_service.md) |
| 13 | **review-service** | reviews, replies, ratings | Kafka (both) | [13](13_review_service.md) |
| 14 | **chat-service** | rooms, messages | WS, Redis, Kafka (both) | [14](14_chat_service.md) |
| 15 | **seller-service** | sellers, KYC, balances | gRPC, Kafka (both) | [15](15_seller_service.md) |
| 16 | **commission-service** | commission ledger, fee rules | Kafka (both) | [16](16_commission_service.md) |
| 17 | **payout-service** | payout batches, transfers | Kafka (both), RabbitMQ | [17](17_payout_service.md) |
| 18 | **storefront-service** | store pages, banners | gRPC, Kafka (cons) | [18](18_storefront_service.md) |

---

## 3. Technology Decisions

| Concern | Choice | Reason |
|---|---|---|
| Language | Go | Low memory, great concurrency for WS hubs + Kafka consumers; matches CashPilotX |
| Router | Chi | Same as CashPilotX — idiomatic, middleware-friendly |
| DB queries | SQLC | Type-safe, compile-time SQL checks |
| DB | PostgreSQL (per service) | Strong constraints, JSONB; isolation prevents coupling |
| Event streaming | **Kafka** | Fan-out, consumer groups, replay, ordered-per-key — the platform backbone |
| Task queue | **RabbitMQ** | Per-message ack, delayed delivery, easy DLQ — for "do this job once" |
| In-service fan-out | **Redis Pub/Sub** | WS room broadcast inside chat/delivery, presence |
| Realtime (bi-dir) | **WebSocket** | Chat, driver GPS upload, live customer map |
| Realtime (push only) | **SSE** | Order status bar, notification bell — simpler than WS |
| Internal RPC | **gRPC** | Typed, fast, generated Go clients between services |
| Cache / session | Redis | Rate limit, cart, token denylist, presence |
| Search | Elasticsearch | Full-text product search across all sellers |
| Object storage | S3 / MinIO | Product photos, KYC docs, review images, delivery proof |
| Auth | PASETO v4 + refresh rotation | No alg-confusion class of JWT bugs; stateless verify at gateway |
| Tracing | OpenTelemetry → Jaeger | Follow one order across all 18 services |
| Metrics | Prometheus + Grafana | Orders/min, Kafka consumer lag, delivery SLA |
| Logs | Loki | Lightweight aggregation, correlated by request-id/trace-id |
| Secrets | Vault | DB creds, channel API keys, Stripe keys |
| Containers | Docker Compose (dev) / K8s (prod) | Same as CashPilotX deploy story |

---

## 4. Kafka Topology — The Event Backbone

### Topic naming & partitioning rules

```
Naming:        <domain>.<event-in-past-tense>          e.g. order.paid
Partition key: the aggregate id that must stay ordered  e.g. sub_order_id, tracking_no
Format:        JSON envelope now; migrate to Avro + Schema Registry post-MVP (⚠ GAP 4)
Retention:     events 7d · delivery.location 24h · compacted topics infinite
```

Every event uses a shared envelope (defined once in `shared/events`):

```json
{
  "event_id":    "uuidv7",
  "event_type":  "order.paid",
  "occurred_at": "2026-06-02T14:30:00Z",
  "trace_id":    "otel-trace-id",
  "version":     1,
  "data":        { /* event-specific payload */ }
}
```

### Topic catalog (who produces, who consumes)

| Topic | Key | Producer | Consumers |
|---|---|---|---|
| `user.registered` | user_id | auth | user, notification |
| `seller.registered` | seller_id | seller | notification |
| `seller.approved` | seller_id | seller | storefront, notification, product |
| `seller.suspended` | seller_id | seller | product, storefront, notification |
| `product.updated` | product_id | product | search, storefront |
| `order.created` | parent_order_id | order | inventory, notification, chat |
| `suborder.created` | sub_order_id | order | inventory, delivery, commission |
| `inventory.reserved` | sub_order_id | inventory | payment, order |
| `inventory.rejected` | sub_order_id | inventory | order, notification |
| `order.paid` | parent_order_id | payment | inventory, delivery, notification, **cashpilotx** |
| `suborder.paid` | sub_order_id | payment | commission, seller, delivery |
| `delivery.checkpoint` | tracking_no | delivery | notification, order |
| `delivery.location` | tracking_no | delivery (driver GPS) | delivery (route builder), WS hub |
| `delivery.status.changed` | tracking_no | delivery | order, notification, **cashpilotx** |
| `order.delivered` | sub_order_id | order | review, payout, user, notification, **cashpilotx** |
| `order.cancelled` | parent_order_id | order | inventory, payment, notification |
| `review.created` | product_id | review | product, storefront |
| `review.flagged` | review_id | review | notification |
| `commission.calculated` | sub_order_id | commission | seller, payout, **cashpilotx** |
| `payout.processed` | payout_id | payout | seller, notification, **cashpilotx** |
| `chat.message.sent` | room_id | chat | notification |

### Consumer-group design (this is the Kafka skill-builder)

- **Independent consumer groups** — `inventory`, `notification`, and `chat` each have their own group on `order.created`; all three get every event independently.
- **Partition by aggregate** — all `delivery.checkpoint` events for one parcel hit the same partition ⇒ the timeline is **strictly ordered**. Same for `suborder.*` keyed by `sub_order_id`.
- **DLQ pattern** — each consumer wraps handling in retry(3); on final failure it publishes to `<topic>.dlq` and commits the offset, so one poison message never blocks the partition.
- **Idempotent consumers** — every handler is keyed on `event_id`; a `processed_events(event_id)` table makes redelivery a no-op (Kafka is at-least-once).
- **Compacted topics** — `product.updated` and `inventory.snapshot` are log-compacted so a fresh search-service can rebuild current state from the topic alone.
- **Exactly-once-ish for money** — payment + commission use the transactional outbox pattern (write DB row + outbox row in one tx; a relay publishes to Kafka) so a charge is never double-emitted.

---

## 5. RabbitMQ vs Kafka vs Redis — When to Use What

```
Kafka     "this happened — whoever cares, react."   broadcast · replayable · ordered-per-key
          order.paid, delivery.checkpoint, review.created

RabbitMQ  "do this one job, exactly once, retry/delay if it fails."
          send-email, send-sms, process-payout, generate-thumbnail, poll-tracking

Redis     "push to live connections / hold ephemeral state right now."
          WS room broadcast (pub/sub), cart, rate-limit window, presence, token denylist
```

**Concrete split inside notification-service:**
`order.paid` arrives on **Kafka** (event) → notification-service decides "send confirmation email"
→ enqueues a **RabbitMQ** `email.send` job (with retry + DLQ) → email worker delivers via SendGrid.
Kafka = the fact; RabbitMQ = the task.

---

## 6. The Multi-Seller Order Flow (end to end)

A customer's cart contains items from **3 different sellers**. This is the heart of the marketplace.

```
1.  Customer checks out cart (items from seller A, B, C)
        │  POST /orders   (api-gateway → order-service)
        ▼
2.  order-service creates:
        parent_order (ORD-001)
          ├── sub_order A  (seller A)
          ├── sub_order B  (seller B)
          └── sub_order C  (seller C)
        → Kafka: order.created  +  suborder.created ×3
        │
        ├─▶ inventory-service consumes suborder.created → reserves stock per seller
        │       → Kafka: inventory.reserved (or inventory.rejected)
        │
        └─▶ notification-service → "Order placed" to customer
                                    + "New order!" to each seller
        │
3.  payment-service waits for all sub_orders' inventory.reserved → charges customer ONCE
        → Kafka: order.paid (parent) + suborder.paid ×3
        │
        ├─▶ inventory-service → converts reservation to deduction
        ├─▶ commission-service → calculates platform fee per sub_order → commission.calculated
        ├─▶ delivery-service → creates a SHIPMENT + tracking_no per sub_order
        ├─▶ cashpilotx → auto-creates an invoice (your financial OS)
        └─▶ notification → "Payment received, sellers preparing"
        │
4.  Each seller ships → driver app scans at each checkpoint
        → Kafka: delivery.checkpoint (per tracking_no, ordered)
        → driver GPS every 30s → Kafka: delivery.location
        │
        ├─▶ delivery-service route builder → polyline for the map
        ├─▶ WS hub → live location pushed to the customer's open map
        ├─▶ notification → "Out for delivery", "Arriving today"
        └─▶ order-service → updates sub_order status
        │
5.  Driver marks delivered → Kafka: delivery.status.changed (delivered)
        → order-service → order.delivered (per sub_order)
        │
        ├─▶ review-service → unlocks review for that product + seller
        ├─▶ payout-service → queues seller payout after hold period (3–7d)
        ├─▶ cashpilotx → marks invoice collected
        └─▶ notification → "Delivered! Rate your purchase"
        │
6.  After hold period → payout-service releases funds → payout.processed
        → seller-service updates balance → notification → "You've been paid"
```

> **Why per-sub_order tracking matters:** the customer sees 3 separate parcels with 3 separate maps
> and timelines (exactly like Shopee when you buy from multiple shops), but paid for them in one charge.

---

## 7. Money Flow & CashPilotX Bridge

```
Customer pays 1,000 THB (one charge, payment-service)
        │
        ▼
payment-service holds full amount, emits suborder.paid per seller
        │
commission-service splits each sub_order:
   sub_order A = 400 THB → fee 8% = 32 → seller earns 368
   sub_order B = 350 THB → fee 8% = 28 → seller earns 322
   sub_order C = 250 THB → fee 8% = 20 → seller earns 230
        │  platform fee total = 80 THB  →  commission.calculated → cashpilotx (revenue)
        │  seller earnings held until delivery + hold window
        ▼
order.delivered (per sub_order) → payout-service starts hold timer (3–7d for returns)
        │
hold expires → payout-service batches transfer → payout.processed
        → seller-service credits balance
        → cashpilotx records outgoing payment to seller
```

| Marketplace event | CashPilotX (admin OS) action |
|---|---|
| `order.paid` | Auto-create customer invoice |
| `commission.calculated` | Record platform-fee revenue |
| `delivery.status.changed` → `delivered` | Mark invoice collected |
| `order.cancelled` (+ refund) | Create credit note |
| `payout.processed` | Record outgoing payment to seller |
| `seller.approved` | Open seller account in ledger |

> **Integration mechanism:** CashPilotX runs a Kafka consumer group `cashpilotx-bridge` subscribed to
> the money/lifecycle topics above. It maps marketplace events onto its existing `CreateInvoiceTx`,
> `RecordPaymentTx`, etc. No direct DB coupling between the two systems. See ⚠ GAP 5.

---

## 8. Cross-Cutting Concerns

### Auth & identity
- **auth-service** issues PASETO v4 access tokens (15 min) + rotating refresh tokens (30 d).
- **api-gateway** verifies the PASETO locally (public key) — no per-request call to auth-service.
- Roles: `customer`, `seller`, `admin`, `driver`. Claims carry `sub`, `role`, `seller_id?`.

### Idempotency
- All client mutations send an `Idempotency-Key` header; gateway + services dedupe (same pattern as CashPilotX).
- All Kafka consumers dedupe on `event_id` via a `processed_events` table.

### Observability
- Every request gets a `request-id`; OTel propagates a `trace-id` across gRPC + Kafka headers.
- Grafana dashboards: orders/min, payment success rate, **Kafka consumer lag per group**, delivery SLA, WS connections.

### Resilience
- gRPC clients wrapped with circuit breaker + retry + timeout (e.g. `sony/gobreaker`).
- RabbitMQ jobs: max-retries → DLQ. Kafka consumers: retry → `<topic>.dlq`.
- Outbox pattern for payment/commission/payout so money events survive a crash between DB commit and publish.

### Security
- Secrets in Vault; no creds in env files in prod.
- Webhook endpoints (payment provider, channel) validate signatures before enqueueing.
- Rate limits at gateway: auth routes, checkout, search.

---

## 9. Repository Layout

```
bazaar/                          # Go workspace (go.work) — or monorepo
├── go.work
├── shared/
│   ├── events/                  # Kafka envelope + all event structs (single source of truth)
│   ├── kafka/                   # producer/consumer wrappers, DLQ, idempotent-consumer helper
│   ├── rabbitmq/                # publisher + worker wrapper, retry/DLQ
│   ├── proto/                   # .proto files + generated gRPC stubs (shared contracts)
│   ├── websocket/               # reusable room/hub manager (used by chat + delivery)
│   ├── middleware/              # PASETO verify, request-id, idempotency, rate-limit
│   ├── otel/                    # tracing + metrics bootstrap
│   └── money/                   # minor-unit money type, currency helpers
├── services/
│   ├── 1-api-gateway/
│   ├── 2-auth/
│   ├── 3-user/
│   ├── 4-product/
│   ├── 5-inventory/
│   ├── 6-cart/
│   ├── 7-order/
│   ├── 8-payment/
│   ├── 9-delivery/
│   ├── 10-notification/
│   ├── 11-search/
│   ├── 12-file/
│   ├── 13-review/
│   ├── 14-chat/
│   ├── 15-seller/
│   ├── 16-commission/
│   ├── 17-payout/
│   └── 18-storefront/
│       ├── cmd/main.go
│       ├── internal/
│       │   ├── api/             # Chi handlers (REST) + gRPC server
│       │   ├── kafka/           # producers + consumers for this service
│       │   ├── service/         # business logic
│       │   └── db/              # SQLC queries + migrations + *_tx.go
│       ├── Dockerfile
│       └── go.mod
├── deploy/
│   ├── docker-compose.yml       # all services + kafka + rabbitmq + redis + pg-per-svc + ES + MinIO
│   └── k8s/
└── docs/                        # ← these files
```

---

## 10. Sprint Plan

```
Sprint 0  — Platform foundation   (Week 1–2)   shared libs, Kafka/Rabbit/Redis up, gateway+auth
Sprint 1  — Catalog & cart        (Week 3–4)   product, inventory, cart, file, search
Sprint 2  — Order pipeline        (Week 5–6)   order state machine + Kafka order.* + payment  ← core Kafka
Sprint 3  — Delivery & tracking   (Week 7–8)   delivery checkpoints + GPS + WS live map       ← core Kafka
Sprint 4  — Multi-seller          (Week 9–10)  seller, commission, payout, storefront         ← marketplace
Sprint 5  — Engagement            (Week 11–12) review, chat, notification fan-out complete
Sprint 6  — CashPilotX bridge     (Week 13)    cashpilotx-bridge consumer, money reconciliation
Sprint 7  — Harden & launch       (Week 14–15) observability, DLQs, load test, K8s deploy
```

### Sprint 0 — Platform foundation
**Goal:** the skeleton everything else plugs into.
- [ ] `go.work` workspace + `shared/` libraries (events, kafka, rabbitmq, websocket, middleware, otel, money)
- [ ] `docker-compose.yml`: Kafka (KRaft), RabbitMQ, Redis, one Postgres per service, Elasticsearch, MinIO, Jaeger, Prometheus, Grafana
- [ ] Shared Kafka producer/consumer wrapper with idempotent-consumer + DLQ helper
- [ ] api-gateway: routing, PASETO verify, rate limit, request-id
- [ ] auth-service: register/login, PASETO issue, refresh rotation → `user.registered`
- [ ] OTel tracing flowing through gateway → auth, visible in Jaeger

### Sprint 1 — Catalog & cart
- [ ] product-service: catalog, variants, categories → `product.updated`
- [ ] inventory-service: stock + reservation model (no order wiring yet)
- [ ] file-service: image upload → MinIO → thumbnail job via RabbitMQ
- [ ] search-service: consume `product.updated` → index in Elasticsearch
- [ ] cart-service: multi-seller cart in Redis

### Sprint 2 — Order pipeline (first big Kafka workout)
- [ ] order-service: parent/sub-order model + state machine → `order.created`, `suborder.created`
- [ ] inventory-service: consume `suborder.created` → reserve → `inventory.reserved`/`inventory.rejected`
- [ ] payment-service: charge once → `order.paid`, `suborder.paid` (outbox pattern)
- [ ] consumer groups + DLQ + idempotent consumers proven end-to-end
- [ ] SSE order status bar (gateway → order-service)

### Sprint 3 — Delivery & tracking (second big Kafka workout)
- [ ] delivery-service: consume `suborder.paid` → create shipment + tracking_no
- [ ] driver API: checkpoint scan → `delivery.checkpoint`; GPS heartbeat → `delivery.location`
- [ ] route builder: aggregate GPS points → polyline (partition-ordered by tracking_no)
- [ ] WebSocket hub: live location → customer map; timeline → customer
- [ ] `delivery.status.changed` → order-service → `order.delivered`

### Sprint 4 — Multi-seller
- [ ] seller-service: onboarding, KYC docs (file-service), `seller.registered`/`approved`/`suspended`
- [ ] product/inventory gain `seller_id`; admin product approval
- [ ] commission-service: consume `suborder.paid` → `commission.calculated`
- [ ] payout-service: consume `order.delivered` → hold timer → `payout.processed` (RabbitMQ delayed job)
- [ ] storefront-service: per-seller store pages

### Sprint 5 — Engagement
- [ ] review-service: consume `order.delivered` → unlock review → `review.created`/`flagged`
- [ ] chat-service: customer ↔ seller WS rooms (Redis pub/sub fan-out), `chat.message.sent`
- [ ] notification-service: consume ALL lifecycle topics → RabbitMQ send jobs (email/SMS/push)

### Sprint 6 — CashPilotX bridge
- [ ] `cashpilotx-bridge` consumer group on `order.paid`, `commission.calculated`, `order.delivered`, `payout.processed`
- [ ] Map events → CashPilotX `CreateInvoiceTx` / `RecordPaymentTx`
- [ ] Reconciliation report: marketplace revenue vs CashPilotX ledger

### Sprint 7 — Harden & launch
- [ ] All consumers have DLQ + alert on lag
- [ ] Load test: checkout + GPS firehose (many drivers × 30s) — tune partitions
- [ ] Grafana dashboards + Jaeger traces complete
- [ ] K8s manifests, secrets in Vault, autoscaling on consumer lag

---

## 11. Open Decisions

These cross-service decisions must be resolved before the sprint they affect.

### ⚠ GAP 1 — Saga orchestration vs choreography for the order flow (before Sprint 2)
The multi-seller flow (reserve → pay → ship → payout) is a distributed transaction.
- **Choreography (events only):** each service reacts to Kafka events. Simpler, looser, but the flow is implicit — hard to see "where is order X stuck?".
- **Orchestration (a saga coordinator):** order-service runs an explicit state machine and issues commands. Clearer, easier compensation (refund/release on failure), but more code.
- **Recommendation:** **orchestration in order-service** for the money-critical path (reserve→pay→deliver→payout) with explicit compensation; choreography for side effects (notifications, search, reviews). Decide before building order-service.

### ⚠ GAP 2 — Payment split: charge-then-split vs split-on-capture (before Sprint 2)
Customer pays once but funds belong to multiple sellers.
- **Charge full to platform, pay out later** (what this doc assumes): simplest, platform holds funds, payout-service disburses. Needs a money-services license in some jurisdictions.
- **Connected accounts (Stripe Connect / provider split):** provider splits at capture. Less regulatory burden but ties you to a provider and may not support SE Asian rails (KBZPay/Wave).
- **Recommendation:** charge-to-platform + payout-service for MVP (works with local rails); revisit Connect if entering regulated markets.

### ⚠ GAP 3 — Driver GPS ingestion volume (before Sprint 3)
GPS every 30s × N active drivers can be a high-volume firehose.
- Options: (a) drivers → REST to delivery-service → Kafka; (b) drivers → Kafka via a thin ingest gateway; (c) batch GPS client-side (send every 60–90s with buffered points).
- Decide partition count for `delivery.location` and whether to downsample server-side before storing the route polyline.
- **Recommendation:** client buffers points, sends every 60s; `delivery.location` partitioned by `tracking_no`; route builder downsamples to 1 point/100m for storage.

### ⚠ GAP 4 — Event schema format: JSON now vs Avro + Schema Registry (before Sprint 5)
JSON envelope is fine to start but has no schema enforcement; as consumers multiply, a producer change can silently break them.
- **Recommendation:** ship JSON for Sprints 0–4; introduce Confluent Schema Registry + Avro (or Protobuf) before Sprint 5 when fan-out is widest. The shared `events/` package makes the migration mechanical.

### ⚠ GAP 5 — CashPilotX coupling boundary (before Sprint 6)
How tightly does the marketplace integrate with the existing CashPilotX admin OS?
- **Option A — Event bridge only (recommended):** CashPilotX adds a `cashpilotx-bridge` Kafka consumer; zero shared DB, zero shared code. Clean, but CashPilotX must learn the marketplace event schema.
- **Option B — Shared `shared/events` module:** both repos import the same Go event structs. Tighter typing, but couples deploy/versioning of two systems.
- **Recommendation:** Option A. Treat CashPilotX as just another consumer on the bus.

### ⚠ GAP 6 — Single-currency vs multi-currency at launch (before Sprint 2)
Sellers and customers may span THB / MMK / USD.
- Money is stored in minor units + currency code from day one (schema-ready). Open question is FX: does the platform settle seller payouts in their local currency, and who bears FX risk?
- **Recommendation:** launch single-currency per marketplace instance (THB for Thailand); design schema multi-currency but defer cross-currency settlement to post-launch.

---

*Master architecture · codename `bazaar` · companion to CashPilotX*
*Per-service detail in `01_…`–`18_…`. Open decisions GAP 1–6 tracked above.*
