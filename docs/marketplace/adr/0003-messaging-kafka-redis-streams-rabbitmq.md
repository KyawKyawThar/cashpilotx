# ADR-0003: Messaging — Kafka log + Redis Streams queue at launch, RabbitMQ in Phase 2

- **Status:** Proposed
- **Date:** 2026-06-02
- **Deciders:** Tech lead (needs your sign-off)
- **Affects:** all event-driven services, ops

## Context

The design calls for Kafka (event log), RabbitMQ (task queue), Redis (cache/pub-sub), Elasticsearch,
gRPC, Vault, WebSocket, and K8s. Introducing all of them at once is ~8 unfamiliar systems to operate
simultaneously — the dominant cause of stalled platforms.

Two of the goals are explicitly **learning goals** (Kafka, RabbitMQ). The team already operates
**Redis Streams** in CashPilotX (proven consumer-group + ack + redelivery semantics). Kafka and a
task queue overlap: a log can drive jobs, and a queue can carry events — but we don't need *both* a
log *and* a separate broker on day one.

## Options considered

### Option A — Kafka + RabbitMQ + Elasticsearch all from day one
Maximum capability, maximum operational surface, slowest path to a working pipeline, highest chance
of debugging multiple new systems at once.

### Option B — Kafka for everything (events *and* jobs), no separate queue
One system to learn. But Kafka is awkward for delayed jobs, per-message retry, and rich routing —
exactly what payouts/notifications need — leading to hacks.

### Option C — Kafka as the event backbone + Redis Streams as the task queue now; add RabbitMQ in Phase 2
Kafka does what it's best at (the learning goal + the fan-out/replay backbone). Redis Streams (already
known) handles launch-volume task queues (email/SMS/push, image processing). RabbitMQ joins in Phase 2
as a deliberate learning milestone *and* when a concrete need appears: delayed payout jobs with rich
routing/DLX. Elasticsearch deferred until Postgres full-text search hits its limit.

## Decision

We will adopt **Option C**:
- **Kafka** = event log / backbone (all `*.created`, `*.paid`, `delivery.*`, etc.). Partitioned by
  aggregate id; consumer groups; DLQ; outbox for money events.
- **Redis Streams** = task queue at launch (notification sends, image processing, hold timers via a
  scheduled sweeper). Reuses CashPilotX operational know-how.
- **RabbitMQ** = Phase 2. Adopt when delayed/routed jobs (payout batching, retry topologies) outgrow
  Redis Streams — and as the planned RabbitMQ learning milestone.
- **Elasticsearch** = Phase 2. Postgres FTS (with `tsvector` + trigram) serves launch search.

Phase 1 thus introduces **two** genuinely new systems (Kafka, WebSocket), not eight.

## Consequences

- Faster, calmer path to a working event pipeline; one new streaming system to master first (Kafka).
- The notification/payout docs that currently say "RabbitMQ" must read "Redis Streams (Phase 1) →
  RabbitMQ (Phase 2)"; the shared `queue` abstraction hides which broker is behind it so the swap is
  a config change.
- We give up RabbitMQ's native delayed-delivery/DLX at launch and emulate delays with a sweeper —
  acceptable at early volume.
- Learning goals are preserved but sequenced: Kafka now, RabbitMQ + Elasticsearch in Phase 2.
