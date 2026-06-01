# Architecture Decision Records (ADRs)

An ADR captures **one architectural decision**, the context that forced it, and the consequences we
accept. We write one whenever a choice is hard to reverse or affects more than one context.

> Why bother: six months from now (or when a new engineer joins), the code shows *what* we did but
> never *why*. ADRs are the "why." They're the difference between architecture and accident.

## Conventions

- One file per decision: `NNNN-short-title.md` (zero-padded, monotonically increasing).
- **Status:** `Proposed` → `Accepted` → (later) `Superseded by ADR-XXXX` / `Deprecated`.
- Never edit an Accepted ADR's decision — write a new ADR that supersedes it.
- Keep them short. A decision, not an essay.
- Use [`template.md`](template.md) to start a new one.

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-modular-monolith-then-extract.md) | Bounded contexts: modular monolith first, extract to services later | Proposed |
| [0003](0003-messaging-kafka-redis-streams-rabbitmq.md) | Messaging stack: Kafka log + Redis Streams queue now, RabbitMQ in Phase 2 | Proposed |
| [0004](0004-saga-orchestration-for-order.md) | Orchestrated saga + outbox for the order/money path | Proposed |
| [0005](0005-database-per-context.md) | Database-per-context, co-located as schemas at launch | Proposed |

> ADRs marked **Proposed** need your sign-off. They encode the recommendations from
> `ARCHITECTURE_PRINCIPLES.md` and the master doc's GAP decisions. Accept, amend, or reject each.
