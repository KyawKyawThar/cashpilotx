# ADR-0004: Orchestrated saga + transactional outbox for the order/money path

- **Status:** Proposed
- **Date:** 2026-06-02
- **Deciders:** Tech lead (needs your sign-off)
- **Affects:** Commerce context (order, payment), inventory, delivery, payout
- **Resolves:** master ⚠ GAP 1

## Context

A multi-seller order is a distributed transaction: reserve stock (per seller) → charge once → create
shipments → release payouts after delivery. Any step can fail and must be compensated (release stock,
refund). Two coordination styles exist: pure event choreography, or an explicit orchestrator.

## Options considered

### Option A — Choreography (services react to each other's events, no coordinator)
Loosely coupled and simple to add reactions. But the end-to-end flow is *implicit* — no single place
answers "where is order X stuck?", and compensation logic is scattered across services.

### Option B — Orchestrated saga (order-service runs an explicit state machine, issues commands)
The flow and its compensations live in one state machine. Easier to reason about, observe, and test;
more code in the orchestrator.

## Decision

We will use **orchestration for the money-critical path** (reserve → pay → ship → deliver → payout),
owned by the Commerce context's order saga, with an explicit compensation for every forward step
(see `ARCHITECTURE_PRINCIPLES.md` §6.1). We will use **choreography for side effects** (notifications,
search indexing, reviews) where order doesn't matter and coupling should stay low.

All events emitted as part of a DB write use the **transactional outbox** so "committed ⇒ published"
holds across crashes.

## Consequences

- One observable state machine answers "where is this order?" and centralizes compensation.
- The order-service carries more logic and is the most important service to test thoroughly
  (testcontainers + contract tests).
- Side-effect consumers stay decoupled and independently deployable.
- Requires the outbox + relay infrastructure in the shared layer before Sprint 2.
