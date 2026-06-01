# Cross-Cutting — Testing Strategy

> How we keep an event-driven, multi-context system trustworthy. Implements
> `ARCHITECTURE_PRINCIPLES.md` §7.6. The goal isn't a coverage number — it's the confidence to
> deploy a single context without breaking the other five.

---

## Table of Contents

1. [Testing Philosophy for a Distributed System](#1-testing-philosophy-for-a-distributed-system)
2. [The Test Pyramid (our shape)](#2-the-test-pyramid-our-shape)
3. [Unit Tests](#3-unit-tests)
4. [Contract Tests — the load-bearing layer](#4-contract-tests--the-load-bearing-layer)
5. [Integration Tests (testcontainers)](#5-integration-tests-testcontainers)
6. [Saga & Compensation Tests](#6-saga--compensation-tests)
7. [End-to-End Tests](#7-end-to-end-tests)
8. [Load & Soak Tests](#8-load--soak-tests)
9. [Chaos / Failure-Injection](#9-chaos--failure-injection)
10. [Test Data & Fixtures](#10-test-data--fixtures)
11. [CI Gates & Coverage Policy](#11-ci-gates--coverage-policy)

---

## 1. Testing Philosophy for a Distributed System

In a monolith, a function call either compiles against the right signature or it doesn't. In an
event-driven system, **the contracts are runtime** (Kafka payloads, gRPC messages) — a producer can
change a field and nothing fails to compile, but a consumer breaks in production.

So our testing budget is spent where the risk actually is:

1. **Contracts between contexts** (events + gRPC) — most important, because breakage is invisible at compile time.
2. **Behavior against real infrastructure** (Postgres, Kafka, Redis) — mocks lie about transactions, ordering, and serialization.
3. **The saga** — the one place with cross-service state and compensation.
4. **The two firehoses** — checkout burst and delivery GPS — because they're the scale risks.

We deliberately write **fewer mock-heavy unit tests** and **more contract + integration tests** than a
typical monolith.

---

## 2. The Test Pyramid (our shape)

```
            ┌───────────────┐
            │   E2E (few)    │  critical user journeys, staging, real services
            ├───────────────┤
            │  Load / Chaos  │  firehoses + failure injection (pre-launch + scheduled)
            ├───────────────┤
        ┌───┴───────────────┴───┐
        │  Integration (many)    │  testcontainers: real PG/Kafka/Redis per service
        ├───────────────────────┤
        │  Contract (many)       │  consumer-driven, events + gRPC — gate cross-context change
        ├───────────────────────┤
   ┌────┴───────────────────────┴────┐
   │  Unit (most, fast)               │  domain logic, pure functions, money math
   └─────────────────────────────────┘
```

Note the **widened contract band** — that's the distributed-system tax, paid deliberately.

---

## 3. Unit Tests

- **Scope:** pure domain logic — order state-machine transitions, commission/rounding math, recovery
  of partial states, validation, the saga decision table (which compensation for which failure).
- **No infrastructure, no mocks of our own DB.** If a test needs Postgres, it's an integration test.
- **Money math is mandatory-unit-tested:** minor-unit arithmetic, fee = gross − earnings exactly,
  rounding rule, multi-seller split summing to the charged total. These are cheap and catch real bugs.
- **Table-driven** (Go idiom), fast (<1s for the whole package).

```go
func TestCommissionSplit(t *testing.T) {
    cases := []struct{ gross int64; rate float64; fee, earn int64 }{
        {40000, 0.08, 3200, 36800},
        {333, 0.08, 27, 306},      // rounding: fee + earnings == gross
    }
    for _, c := range cases { /* assert fee+earn == gross */ }
}
```

---

## 4. Contract Tests — the load-bearing layer

The defense against the distributed monolith. Two kinds.

### 4.1 Event contracts (consumer-driven)

For every event in `EVENT_CATALOG.md`, the **producer** and **each named consumer** share a contract:

- The **producer test** asserts it emits an envelope that matches the published schema (`shared/events`).
- Each **consumer test** asserts it can decode and handle that schema, including **forward-compat**
  (an unknown extra field must not break it) and **idempotency** (same `event_id` twice = one effect).
- A change to a `shared/events` struct **runs all contract tests for that event** in CI. A breaking
  change fails the build until either the change is made additive or all consumers are updated +
  versioned (`EVENT_CATALOG.md` §5).

```
shared/events/order_paid.go changed
   → CI runs: producer(commerce) contract  +  consumer(inventory|delivery|notification|cashpilotx) contracts
   → any decode/handle failure ⇒ red build
```

Tooling: a lightweight in-repo contract harness (golden envelope JSON per event + version) is enough
at launch; Pact/Schema-Registry compatibility checks come with Avro in Phase 2.

### 4.2 gRPC contracts

- Proto compatibility check in CI (`buf breaking`) — no field renumbering/removal without a version.
- Each gRPC server has a contract test per method; clients test against the generated stubs.

> **Why consumer-driven:** the producer can't know all consumers, but the catalog does. Each consumer
> contributes the slice of the schema it depends on, so the producer learns *exactly* what it may not break.

---

## 5. Integration Tests (testcontainers)

**Real infrastructure, no infra mocks.** Each service spins up its real dependencies in containers.

| Dependency | Why a mock would lie |
|---|---|
| **PostgreSQL** | Transactions, constraints, `FOR UPDATE` row locks (inventory last-unit race), JSONB — mocks don't replicate them |
| **Kafka** | Partition ordering, consumer-group rebalancing, offset commit semantics, redelivery |
| **Redis** | Stream consumer groups (task queue), pub/sub fan-out, TTL/eviction |

What we assert:
- **Outbox → publish:** write in a tx, kill the relay mid-flight, restart → event still published exactly as designed.
- **Idempotent consumer:** deliver the same `event_id` twice → one DB effect.
- **Inventory race:** two concurrent `ReserveStockTx` on the last unit → exactly one succeeds (`FOR UPDATE`).
- **Migrations apply cleanly** from empty on every CI run (per-context migration dir).
- **Ordering:** checkpoints for one `tracking_no` consumed in `occurred_at` order.

```go
func TestReserveLastUnit_Concurrent(t *testing.T) {
    pg := testcontainers.StartPostgres(t)          // real PG
    // seed stock on_hand=1; fire 2 concurrent reservations
    // assert: exactly one inventory.reserved, one inventory.rejected
}
```

Integration tests run per-service in CI and are the **primary** confidence layer.

---

## 6. Saga & Compensation Tests

The order saga (ADR-0004) is the riskiest logic — cross-service state with compensations. It gets a
dedicated suite that drives the orchestrator through every branch using real Kafka + Postgres:

| Scenario | Expected |
|---|---|
| Happy path | reserve → pay → ship → deliver → payout, parent status derived correctly |
| One sub-order out of stock | `inventory.rejected` → release others → cancel order → (refund if charged) |
| Payment fails after reservation | release all reservations → `order.cancelled` |
| Crash mid-saga (kill orchestrator) | on restart, saga resumes from persisted state (no double charge) |
| Duplicate `order.paid` | idempotent — one set of downstream effects |
| Refund after payout queued | hold clawed back; no double payout |

Every forward step's compensation in `ARCHITECTURE_PRINCIPLES.md` §6.1 has at least one test.

---

## 7. End-to-End Tests

Few, expensive, high-value. Run against a full stack (docker-compose / staging) over the **critical
journeys** only:

1. Register → browse → add multi-seller cart → checkout → pay → see 3 trackings.
2. Driver scans checkpoints + sends GPS → customer sees timeline grow + pin move (WS) → delivered.
3. Delivered → review unlocked → review posted → product rating updates.
4. Delivered → hold elapses → payout processed → seller balance updated → cashpilotx receives events.

E2E asserts the **seams** work together; it does not re-test branch logic (that's lower layers).

---

## 8. Load & Soak Tests

Targeting the two known scale risks (master ⚠ GAP 3), run before launch and on a schedule:

- **Checkout burst:** N concurrent multi-seller checkouts → measure order-saga latency, payment
  success rate, Kafka consumer lag, DB connection saturation. SLO: checkout success ≥ 99.5%.
- **Delivery GPS firehose:** simulate M active drivers × points every 60s → tune
  `delivery.location` partition count, producer batch/linger, route-builder downsampling, WS hub
  fan-out. Verify no consumer lag growth (soak for hours).
- **Search load:** query mix against the catalog index (Postgres FTS at launch).

Tools: k6 / vegeta for HTTP, a custom Go producer for the Kafka firehose. Results recorded as an ADR
input for partition counts.

---

## 9. Chaos / Failure-Injection

Phase 2, but designed for now. Verify resilience claims are real:

- Kill a broker partition leader mid-stream → consumers recover, no lost events (outbox + at-least-once).
- Kill a service mid-saga → resume correctly.
- Inject gRPC latency/errors → circuit breaker opens, no cascade.
- Fill a DLQ → alert fires; replay tooling drains it.

---

## 10. Test Data & Fixtures

- **Factories, not fixtures-files** — Go builders (`NewSeller()`, `NewOrder(WithSubOrders(3))`) keep
  tests readable and resilient to schema change.
- **Seed data** for local dev (a few sellers, products, a driver) via a `make seed` against compose.
- **No production data in tests/dev.** PII/KYC never copied down (`ARCHITECTURE_PRINCIPLES.md` §7.2);
  synthetic generators only.
- **Deterministic time/ids** in tests (injectable clock + uuid source) so saga timers and outbox
  ordering are assertable.

---

## 11. CI Gates & Coverage Policy

A PR merges only if:

| Gate | Requirement |
|---|---|
| Unit + integration | green (per changed context) |
| Contract tests | green for any touched event/proto (auto-selected from `shared/events` diff) |
| `buf breaking` | no breaking proto change without version bump |
| Migrations | apply cleanly from empty |
| Lint | enforces the non-negotiable rules where checkable (no cross-context import, money-as-int) |
| Coverage | **domain/saga/money packages ≥ 85%**; overall tracked but not gated on a vanity number |

**Coverage is a tool, not a target.** We gate the high-risk packages (saga, money, ownership checks)
hard; we don't chase 100% on glue code. A green contract + integration suite is worth more than a
coverage percentage.

### Pipeline shape

```
PR  → lint + unit (fast, <2min)  → integration (testcontainers, parallel per service)
    → contract tests (selected by diff)  → build images
merge to main → deploy to staging → smoke E2E (journeys 1–4) → canary to prod (money paths)
nightly → full E2E + load + (Phase 2) chaos
```

---

*Cross-cutting · implements `ARCHITECTURE_PRINCIPLES.md` §7.6 · pairs with `EVENT_CATALOG.md` (contracts) and ADR-0004 (saga)*
