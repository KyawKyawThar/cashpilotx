# Cross-Cutting — Event Catalog & Governance

> The single source of truth for every Kafka event on the platform. Implements
> `ARCHITECTURE_PRINCIPLES.md` §6.5. If an event isn't in this catalog, it doesn't exist.
> The shared `shared/events` Go package is the code mirror of this document.

---

## Table of Contents

1. [Why a Catalog](#1-why-a-catalog)
2. [The Envelope](#2-the-envelope)
3. [Event Kinds & Naming](#3-event-kinds--naming)
4. [Ownership & the Golden Rule](#4-ownership--the-golden-rule)
5. [Versioning & Compatibility](#5-versioning--compatibility)
6. [Topic, Key & Retention Conventions](#6-topic-key--retention-conventions)
7. [The Catalog](#7-the-catalog)
8. [Event Schemas (payloads)](#8-event-schemas-payloads)
9. [Delivery Guarantees & Idempotency](#9-delivery-guarantees--idempotency)
10. [DLQ & Poison Messages](#10-dlq--poison-messages)
11. [Governance Process](#11-governance-process)

---

## 1. Why a Catalog

With ~20 event types and many consumers, an ungoverned event bus rots into a "distributed monolith of
events" — producers change a field and silently break consumers they've never heard of. The catalog
+ the shared `events` package + contract tests (see `TESTING_STRATEGY.md`) prevent that.

---

## 2. The Envelope

**Every** event, on every topic, is wrapped in this envelope. Consumers can rely on it unconditionally.

```json
{
  "event_id":    "uuidv7",                 // unique; consumers dedupe on this
  "event_type":  "order.paid",             // matches the catalog
  "occurred_at": "2026-06-02T14:30:00Z",   // when the fact happened (not when published)
  "trace_id":    "otel-trace-id",          // propagated for distributed tracing
  "version":     1,                        // schema version of `data`
  "producer":    "commerce",               // context/service that emitted it
  "actor":       { "id": "uuid", "role": "customer" },  // who caused it (audit), nullable for system
  "data":        { /* event-specific, versioned payload */ }
}
```

Go (`shared/events`):

```go
type Envelope[T any] struct {
    EventID    uuid.UUID `json:"event_id"`
    EventType  string    `json:"event_type"`
    OccurredAt time.Time `json:"occurred_at"`
    TraceID    string    `json:"trace_id"`
    Version    int       `json:"version"`
    Producer   string    `json:"producer"`
    Actor      *Actor    `json:"actor,omitempty"`
    Data       T         `json:"data"`
}
```

> **Format:** JSON at launch. Migrate to Avro/Protobuf + Schema Registry in Phase 2 (master ⚠ GAP 4,
> ADR to follow). The envelope + `shared/events` make that migration mechanical.

---

## 3. Event Kinds & Naming

Naming: **`<domain>.<thing>.<past-tense>`** or `<domain>.<past-tense>`. Always past tense — an event
is a fact that already happened, never a command.

| Kind | Purpose | Coupling | Example |
|---|---|---|---|
| **Notification** (thin) | "Something happened, go look if you care." Minimal payload. | low | `review.flagged` |
| **State transfer** (fat) | Carries the data consumers need so they don't call back. | medium | `product.updated`, `order.paid` |
| **Ledger** (event-sourced) | Append-only financial truth; replayable. Reserved for money. | n/a | commission entries |

✅ `order.paid`, `delivery.checkpoint`, `seller.approved`
❌ `pay_order`, `send_email`, `OrderPaidEvent`, `updateInventory` (those are commands/jobs, not events)

Commands (do-this-now) and jobs (do-this-later) are **not** Kafka events — they're gRPC calls or
queue messages (Redis Streams / RabbitMQ). Don't put `email.send` on Kafka.

---

## 4. Ownership & the Golden Rule

**Each event is owned by exactly one context — the one whose state change it represents.** Only the
owner produces it. Anyone may consume it.

> **Golden rule:** a service publishes events about **its own** data only. inventory-service never
> publishes `order.paid`; payment-service does. If you find yourself wanting to publish another
> context's event, you have a boundary problem.

| Owning context | Events it (and only it) produces |
|---|---|
| Identity | `user.*` |
| Catalog | `product.*`, `inventory.*` |
| Commerce | `order.*`, `suborder.*`, `payment.*` |
| Fulfillment | `delivery.*` |
| Marketplace | `seller.*`, `commission.*`, `payout.*` |
| Engagement | `review.*`, `chat.*` |

---

## 5. Versioning & Compatibility

Consumers outnumber producers and deploy independently. Therefore:

- **Additive-only within a version.** Adding an **optional** field → same version, consumers ignore
  unknowns. This is the common case.
- **Breaking change → new version**, never an edit. A removed/renamed/retyped field, or a new
  required field, bumps `version` and (if needed) publishes to a `vN` topic suffix or dual-writes
  during migration.
- **Producers dual-publish** old + new version during a migration window; consumers migrate; old
  version retired only when no consumer reads it (tracked here in the catalog).
- **Never reuse a field name** with a new meaning.
- **The `shared/events` package is the contract.** A change there triggers consumer-driven contract
  tests (`TESTING_STRATEGY.md` §4) in CI — a breaking change fails the build until consumers are ready.

---

## 6. Topic, Key & Retention Conventions

| Concern | Rule |
|---|---|
| Topic name | == event type (`order.paid` → topic `order.paid`) |
| Partition key | the aggregate id that must stay ordered (see catalog) |
| Ordering | guaranteed **per key**, not globally |
| Default retention | 7 days |
| High-volume retention | `delivery.location` → 24h |
| Compacted (infinite) | `product.updated`, `inventory.snapshot` — rebuildable read models |
| DLQ | `<topic>.dlq` per consumer group |

---

## 7. The Catalog

| Event | v | Key | Owner | Kind | Consumers |
|---|---|---|---|---|---|
| `user.registered` | 1 | user_id | Identity | state | user, notification |
| `user.suspended` | 1 | user_id | Identity | notif | (various) |
| `product.updated` | 1 | product_id | Catalog | state (compacted) | search, storefront |
| `inventory.reserved` | 1 | sub_order_id | Catalog | state | order(payment trigger) |
| `inventory.rejected` | 1 | sub_order_id | Catalog | notif | order, notification |
| `inventory.snapshot` | 1 | sku | Catalog | state (compacted) | search, cashpilotx |
| `order.created` | 1 | parent_order_id | Commerce | state | inventory, notification, chat |
| `suborder.created` | 1 | sub_order_id | Commerce | state | inventory, delivery, commission |
| `order.paid` | 1 | parent_order_id | Commerce | state | inventory, delivery, notification, **cashpilotx** |
| `suborder.paid` | 1 | sub_order_id | Commerce | state | commission, seller, delivery |
| `payment.failed` | 1 | parent_order_id | Commerce | notif | order |
| `payment.refunded` | 1 | parent_order_id | Commerce | state | commission, payout, **cashpilotx** |
| `order.cancelled` | 1 | parent_order_id | Commerce | state | inventory, payment, notification |
| `order.delivered` | 1 | sub_order_id | Commerce | state | review, payout, user, notification, **cashpilotx** |
| `delivery.checkpoint` | 1 | tracking_no | Fulfillment | state | notification, order |
| `delivery.location` | 1 | tracking_no | Fulfillment | state (24h) | delivery(route), WS hub |
| `delivery.status.changed` | 1 | tracking_no | Fulfillment | state | order, notification, **cashpilotx** |
| `seller.registered` | 1 | seller_id | Marketplace | notif | notification |
| `seller.approved` | 1 | seller_id | Marketplace | state | storefront, product, notification |
| `seller.suspended` | 1 | seller_id | Marketplace | state | product, storefront, notification |
| `commission.calculated` | 1 | sub_order_id | Marketplace | ledger | seller, payout, **cashpilotx** |
| `commission.reversed` | 1 | sub_order_id | Marketplace | ledger | seller, **cashpilotx** |
| `payout.processed` | 1 | payout_id | Marketplace | state | seller, notification, **cashpilotx** |
| `review.created` | 1 | target_id | Engagement | state | product, storefront |
| `review.flagged` | 1 | review_id | Engagement | notif | notification |
| `chat.message.sent` | 1 | room_id | Engagement | notif | notification |

> `cashpilotx` consumes via the external `cashpilotx-bridge` group (master ⚠ GAP 5) — no shared DB.

---

## 8. Event Schemas (payloads)

The `data` field per event. (Representative; the authoritative copy is `shared/events` Go structs.)

```jsonc
// order.created (v1)
{ "parent_order_id":"uuid", "user_id":"uuid", "currency":"THB",
  "total_minor": 100000, "sub_orders":[{"sub_order_id":"uuid","seller_id":"uuid","subtotal_minor":40000}] }

// suborder.created (v1)
{ "sub_order_id":"uuid", "parent_order_id":"uuid", "seller_id":"uuid",
  "items":[{"sku":"TS-RED-M","qty":2,"unit_price_minor":25000}] }

// inventory.reserved (v1)
{ "sub_order_id":"uuid", "ok":true, "items":[{"sku":"TS-RED-M","qty":2}] }

// inventory.rejected (v1)
{ "sub_order_id":"uuid", "ok":false, "sku":"TS-RED-M", "reason":"out_of_stock" }

// order.paid (v1)
{ "parent_order_id":"uuid", "payment_id":"uuid", "amount_minor":100000, "currency":"THB",
  "splits":[{"sub_order_id":"uuid","seller_id":"uuid","gross_minor":40000}] }

// suborder.paid (v1)
{ "sub_order_id":"uuid", "seller_id":"uuid", "gross_minor":40000, "currency":"THB" }

// delivery.checkpoint (v1)
{ "tracking_no":"TH268816827517W", "sub_order_id":"uuid", "status":"out_for_delivery",
  "description":"Parcel is in the process of being delivered",
  "location_name":"ATHON-B - ธนบุรี", "lat":13.7234, "lng":100.4567, "occurred_at":"..." }

// delivery.location (v1)  — batched points
{ "tracking_no":"TH268816827517W", "points":[{"lat":13.72,"lng":100.45,"recorded_at":"..."}] }

// delivery.status.changed (v1)
{ "tracking_no":"TH268816827517W", "sub_order_id":"uuid", "old":"out_for_delivery", "new":"delivered" }

// commission.calculated (v1)
{ "sub_order_id":"uuid", "seller_id":"uuid", "gross_minor":40000, "rate":0.08,
  "fee_minor":3200, "earnings_minor":36800, "currency":"THB" }

// payout.processed (v1)
{ "payout_id":"uuid", "seller_id":"uuid", "amount_minor":36800, "currency":"THB",
  "period_start":"2026-05-26", "period_end":"2026-06-01" }

// review.created (v1)
{ "review_id":"uuid", "target":"product", "target_id":"uuid", "seller_id":"uuid",
  "user_id":"uuid", "rating":5 }

// seller.approved (v1)
{ "seller_id":"uuid", "store_slug":"acme", "commission_rate":0.08 }
```

---

## 9. Delivery Guarantees & Idempotency

- **At-least-once.** Kafka + the outbox relay guarantee delivery, never exactly-once on the wire.
  Therefore **every consumer must be idempotent**.
- **Dedupe on `event_id`.** Each consuming service keeps a `processed_events(event_id)` table; a
  re-delivered event is a no-op. (Mandated in `ARCHITECTURE_PRINCIPLES.md` §8 rule 3.)
- **Producers use the outbox** for any event emitted with a DB write (money/stock/order) so a crash
  between commit and publish can't lose the event.
- **Ordering** is per partition key only. Design consumers to tolerate unrelated events arriving in
  any order; rely on the key for the things that must be ordered (a parcel's checkpoints).

---

## 10. DLQ & Poison Messages

```
consume → handle (retry ×3 with backoff)
            success → commit offset
            permanent failure → publish to <topic>.dlq (with error + original envelope)
                              → commit offset  (so one poison msg never blocks the partition)
DLQ is monitored; alert on depth > 0. Reprocess after fixing the bug (replay tooling in shared/kafka).
```

A malformed `delivery.checkpoint` from a buggy driver app goes to `delivery.checkpoint.dlq` — the
parcel's other checkpoints keep flowing.

---

## 11. Governance Process

Adding or changing an event:

1. **Propose** — add the event to §7 + a schema to §8 in a PR; add the Go struct to `shared/events`.
2. **Classify** — kind (notification/state/ledger), owner context, key, retention.
3. **Compatibility check** — additive (same version) or breaking (new version + migration plan)?
4. **Contract tests** — producer + each named consumer get/extend a contract test (`TESTING_STRATEGY.md` §4).
5. **Review** — owning-context lead + one consumer-context lead approve.
6. **Ship** — producer first (consumers tolerate unknown events), then consumers.

Retiring an event: confirm zero consumers in the catalog → mark `Deprecated` with a removal date →
remove after the window.

> **One rule above all:** the catalog and `shared/events` move together. A code event with no catalog
> entry (or vice-versa) fails review.

---

*Cross-cutting · implements `ARCHITECTURE_PRINCIPLES.md` §6.5 · mirrored by `shared/events` · see `00_system_architecture.md` §4*
