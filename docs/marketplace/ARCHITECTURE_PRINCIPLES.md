# CashPilotX Marketplace — Architecture Principles & Evolution

> Read this **before** `00_system_architecture.md` and the service docs.
> The service docs describe the *target* (18 services). This doc describes *how we get there
> safely* and the rules every service must obey. Strong architecture = clear boundaries +
> stable contracts + a deliberate evolution path — **not** a large service count on day one.

---

## Table of Contents

1. [First Principles](#1-first-principles)
2. [The Big Risk We Are Avoiding: The Distributed Monolith](#2-the-big-risk-we-are-avoiding-the-distributed-monolith)
3. [Bounded Contexts (the real structure)](#3-bounded-contexts-the-real-structure)
4. [Deployment Evolution — Modular Monolith → Services](#4-deployment-evolution--modular-monolith--services)
5. [The Innovation Budget](#5-the-innovation-budget)
6. [Core Patterns (named & governed)](#6-core-patterns-named--governed)
7. [Cross-Cutting Pillars](#7-cross-cutting-pillars)
8. [Non-Negotiable Rules](#8-non-negotiable-rules)
9. [What Strong Architecture Buys Us](#9-what-strong-architecture-buys-us)

---

## 1. First Principles

1. **Boundaries over services.** We invest in *where the seams are*, not in how many processes we run. A clean boundary can live in a package today and a separate deployable tomorrow with no rewrite.
2. **Contracts are the architecture.** Events, gRPC protos, and DB-per-context are the durable artifacts. Code behind a contract is replaceable; the contract is not (it's versioned).
3. **Evolvability beats completeness.** We optimize for "easy to change" over "feature-complete on paper." Defer anything that can be added behind a stable contract.
4. **Spend novelty deliberately.** Every new technology costs operational attention. We add them on a schedule, never all at once (see §5).
5. **Design for the team we have.** Conway's Law is real: the architecture will mirror the org. A small team ships a few well-bounded deployables far better than 18 micro-deployables.
6. **Consistency where it matters, eventual everywhere else.** Money and stock paths get strong guarantees (saga + outbox). Read models (search, storefront, feeds) are eventually consistent on purpose.

---

## 2. The Big Risk We Are Avoiding: The Distributed Monolith

The failure mode for a project like this is **not** "the monolith got too big." It's:

> Splitting into 18 services on day one, then discovering that placing one order requires
> synchronous calls across six of them. Now you have a monolith's coupling **plus** network
> failures, distributed transactions, 18 deploy pipelines, and undebuggable latency.

A distributed monolith is strictly worse than a monolith. We avoid it with three rules:

- **No synchronous call chains for writes.** A write path may touch at most one service synchronously; everything else reacts via events. (The order saga is asynchronous by design.)
- **No shared database — ever.** Cross-context data is obtained via gRPC (read now) or events (react later), never by reaching into another context's tables.
- **A context must deploy without coordinating with another.** If shipping Catalog requires shipping Commerce at the same time, the boundary is wrong — fix the contract.

If those three hold, we can deploy as 1, 6, or 18 processes and it's an *operational* choice, not an architectural one.

---

## 3. Bounded Contexts (the real structure)

The 18 services are **modules inside 6 bounded contexts**. The context is the unit of ownership,
the database boundary, and the deployment unit (initially).

| Context | Modules (the "18 services") | Owns | Consistency |
|---|---|---|---|
| **Identity & Access** | auth, user | credentials, profiles, addresses | strong |
| **Catalog** | product, inventory, search, file | products, stock, index, media | strong writes, eventual read models |
| **Commerce** | cart, order, payment | orders, the order saga, charges | strong (saga + outbox) |
| **Fulfillment** | delivery (+ driver) | shipments, checkpoints, GPS, live map | ordered-per-parcel (Kafka key) |
| **Marketplace** | seller, commission, payout, storefront | sellers, fees, payouts, store pages | strong money + eventual storefront |
| **Engagement** | review, chat, notification | reviews, messages, notifications | eventual |

**Why these six:** each context has a single language (a "product" means the same thing inside
Catalog everywhere), a clear owner, and a minimal, stable surface to the others (a handful of events
+ a few gRPC calls). Cross-context chatter is low; intra-context calls are cheap (in-process).

> The CashPilotX admin OS is a **seventh, external context** that consumes our money/lifecycle
> events. It is never called synchronously and never shares our databases (master ⚠ GAP 5).

---

## 4. Deployment Evolution — Modular Monolith → Services

We build the **boundaries** of all 18 modules from day one, but we **deploy 6 context-services**
(a "modular monolith per context"). We split a context into its sub-services only when a real
signal demands it.

```
PHASE 1  (launch)         6 deployables, one per bounded context.
                          Modules are Go packages inside a context binary.
                          Intra-context calls are function calls (fast, transactional).
                          Inter-context calls are gRPC + Kafka from day one.

PHASE 2  (scale signals)  Split a module out when ANY of these fire:
                          • it needs to scale independently (delivery GPS firehose)
                          • it has a different availability/latency profile (search, chat)
                          • a separate team now owns it (Conway)
                          • its deploy cadence diverges sharply from its context

PHASE 3  (full mesh)      Up to 18 services. No code rewrite — a module already had its own
                          schema, its own gRPC surface, and its own event contracts. Splitting =
                          move the package into its own binary + point the gRPC client at the network.
```

**The litmus test for "is this module ready to be its own service?"** — it already (a) owns its
tables with no cross-module FK, (b) exposes a gRPC/event contract its context-mates use, and (c) has
its own migrations. If yes, extraction is mechanical.

**First services we'd actually split (predicted):** `delivery` (firehose + WebSocket scaling) and
`search` (Elasticsearch lifecycle differs from everything else). Likely in Phase 2 / Sprint 7.

> This is "monolith-first" / "start with a modular monolith" (Fowler, Newman). It is the
> single most important decision in this doc. **Microservices are an extraction target, not a
> starting point.**

---

## 5. The Innovation Budget

Each unfamiliar technology is an "innovation token" we must pay to operate. We have a finite budget
per phase. The learning goals (Kafka, RabbitMQ, WebSocket) are **real and kept** — but *sequenced*
so we're never debugging five new systems at once.

| Technology | When introduced | Why then | Token cost |
|---|---|---|---|
| Go, Postgres, sqlc, chi, Redis | Phase 1, day 1 | Team already runs these in CashPilotX — **free** | 0 |
| **Kafka** | Phase 1, Sprint 2 | The primary learning goal + the event backbone | high |
| **WebSocket/SSE** | Phase 1, Sprint 3 | Live delivery map + chat — core UX | medium |
| gRPC | Phase 1, Sprint 1 | Typed inter-context calls; small surface to start | medium |
| Elasticsearch | **Phase 2** | Postgres full-text search is enough at launch volume | high — **deferred** |
| **RabbitMQ** | **Phase 2** | Redis Streams (already known) covers task queues at launch; adopt Rabbit when delayed/routing needs exceed it — and as a deliberate learning milestone | medium — **deferred** |
| Vault | Phase 2 | Start with sealed secrets / SOPS; Vault when team grows | medium — **deferred** |
| Kubernetes | Phase 2 | Docker Compose → a single small cluster; full K8s + autoscaling later | high — **deferred** |

> **Key call (see ADR-0003):** for launch, **Kafka is the event log** and **Redis Streams is the
> task queue** (we already operate it). RabbitMQ joins in Phase 2 as a learning milestone *and* when
> a concrete need appears (delayed payout jobs with rich routing). We do **not** run a log and a
> separate broker on day one. Elasticsearch waits until Postgres FTS hits its limit.

This keeps Phase 1 to **two genuinely new systems (Kafka, WebSocket)** instead of eight.

---

## 6. Core Patterns (named & governed)

These patterns appear in the service docs; here they are named so the team uses them consistently.

### 6.1 Saga (orchestrated) — the order/money path
The reserve → pay → ship → deliver → payout flow is a **saga orchestrated by the Commerce context**.
Every forward step has a defined **compensation**:

| Forward step | Compensation on failure |
|---|---|
| reserve stock | release reservation |
| charge customer | refund |
| create shipment | cancel shipment |
| start payout hold | claw back / cancel hold |

The orchestrator owns an explicit state machine so "where is order X stuck?" is answerable from one
place (master ⚠ GAP 1).

### 6.2 Transactional Outbox — never lose a money event
Any service that emits an event as part of a DB write does it via an **outbox table** in the same
transaction; a relay publishes to Kafka. Guarantees "DB committed ⇒ event will be published," even
across crashes. Mandatory for order, payment, commission, payout.

### 6.3 CQRS read models — search & storefront
`search-service` and `storefront-service` are **read projections** rebuilt from compacted Kafka
topics. They are disposable: delete and replay to rebuild. They never accept writes.

### 6.4 Idempotency — everywhere, two layers
- **Inbound (clients):** `Idempotency-Key` header dedupes mutations (same as CashPilotX).
- **Inbound (consumers):** every Kafka consumer dedupes on `event_id` via `processed_events`.

### 6.5 Event taxonomy — avoid coupling
Three event kinds, used deliberately:
- **Notification event** (thin): "something happened, go look" — `review.flagged`. Low coupling.
- **Event-carried state transfer** (fat): carries the data consumers need so they don't call back — `product.updated`, `order.paid`. Reduces synchronous chatter.
- **We do _not_ event-source the whole system.** Event sourcing is reserved for audit-critical
  ledgers (commission), not general state.

---

## 7. Cross-Cutting Pillars

These are owned by the platform/shared layer and apply to every context. Each becomes its own ADR.

### 7.1 Authorization
- **Coarse at the gateway:** authenticated + role (`customer/seller/admin/driver`).
- **Fine-grained in the context:** resource-ownership checks ("does seller A own product X?") live
  with the data, never at the edge. RBAC at launch; ABAC/policy engine (OPA) only if rules explode.

### 7.2 PII, PCI & KYC (compliance is architectural, not a feature)
- **Never store card PANs.** Tokenize via the payment provider; we keep only a provider token.
  This keeps most services **out of PCI-DSS scope**.
- **Encrypt at rest:** bank account numbers (seller), KYC docs (private bucket, signed URLs),
  customer PII. Reuse the CashPilotX encryption-key approach.
- **Data lifecycle:** retention windows + a deletion path (right-to-erasure) designed in, not bolted on.
- **PCI scope isolation:** only `payment-service` touches provider card flows; everything else sees tokens/refs.

### 7.3 Multi-tenancy (sellers are tenants)
- Seller data is tenant-scoped; queries always filter by `seller_id`.
- Plan for noisy-neighbor (one huge seller) at the partition/index level (Kafka key, ES routing).

### 7.4 Fraud & abuse (marketplace-specific)
Designed-in hooks, even if rules are simple at launch: fake-order/velocity checks, review-fraud
(verified-purchase gating already helps), seller collusion, chargeback handling. A future
`risk-service` consumes the event bus read-only — no new write coupling.

### 7.5 Observability & SLOs
- **Golden signals** per service: latency, traffic, errors, saturation. **Kafka consumer lag** is a
  first-class SLI.
- OTel traces span gRPC **and** Kafka (trace-id in event headers).
- Define **SLOs + error budgets** before launch (e.g. checkout success ≥ 99.5%, tracking read p99 < 300ms).

### 7.6 Testing strategy
- **Unit** — domain logic.
- **Integration** — testcontainers (real Postgres/Kafka/Redis), no mocks for infra.
- **Contract** — consumer-driven contracts for events + gRPC (a producer change can't silently break consumers).
- **Load** — the two firehoses: checkout burst and delivery GPS (master ⚠ GAP 3) — before launch.

### 7.7 Delivery (CI/CD)
- One pipeline template per context; trunk-based + feature flags; progressive delivery (canary) for
  money paths; DB migrations gated and backward-compatible (expand/contract).

### 7.8 DR & data
- DB-per-context backups with defined **RPO/RTO**; Kafka topic replication; multi-AZ in Phase 2.
- Compacted topics double as rebuild sources for read models.

### 7.9 Developer experience
- A dev must **not** run 18 services to work on one. `docker-compose` profiles bring up only the
  context under work + its dependencies (Kafka/Redis/Postgres). Phase 2: Tilt/Skaffold.
- A **service template / golden path** (the `18-storefront` skeleton in the master doc) so every new
  module starts identical: handlers, gRPC, kafka, db, migrations, otel wired.

### 7.10 Cost awareness
Co-locate Postgres databases as **separate schemas in one cluster** at launch (still logically
DB-per-context, no cross-schema FK) to avoid paying for 18 instances. Split to dedicated clusters in
Phase 2 when a context's load justifies it.

---

## 8. Non-Negotiable Rules

Every PR and every service is held to these. (Promote each to a lint/CI check where possible.)

1. **No cross-context database access.** gRPC or events only.
2. **No cross-context FK.** Foreign keys stop at the context boundary.
3. **Every mutating endpoint accepts `Idempotency-Key`.** Every consumer dedupes on `event_id`.
4. **Money/stock events go through the outbox.** Never publish-then-commit.
5. **Events follow the shared envelope** (`event_id, event_type, occurred_at, trace_id, version, data`) and are versioned — never break a field; add a new version.
6. **No card PAN in our databases.** Tokens only.
7. **Money is integer minor units + currency.** Never floats.
8. **Every service emits the golden signals + consumer lag** and propagates `trace-id`.
9. **A context deploys independently.** If it can't, the boundary is broken — fix the contract, don't add a workaround.
10. **Decisions are recorded as ADRs.** No silent architecture changes.

---

## 9. What Strong Architecture Buys Us

If we hold the boundaries and contracts:

- **We can start small (6 deployables) and grow to 18 with zero rewrites** — extraction is mechanical.
- **We can defer Elasticsearch, RabbitMQ, Vault, K8s** without repainting ourselves into a corner.
- **We can swap implementations** behind a contract (e.g. change the payment provider) without touching consumers.
- **CashPilotX integrates as just another consumer** — no coupling between the two systems.
- **New engineers onboard into one context**, not the whole platform.

That is the payoff for designing the architecture first and letting the services follow.

---

*Foundational doc · read before `00_system_architecture.md` · decisions tracked in `adr/`*
