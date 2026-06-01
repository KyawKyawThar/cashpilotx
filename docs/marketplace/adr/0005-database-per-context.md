# ADR-0005: Database-per-context, co-located as schemas at launch

- **Status:** Proposed
- **Date:** 2026-06-02
- **Deciders:** Tech lead (needs your sign-off)
- **Affects:** all contexts, data, cost

## Context

"Database-per-service" is the textbook microservices rule and prevents hidden coupling. Taken
literally for 18 services it means 18 database instances — real money and ops overhead before we
have load to justify it. We want the *isolation* of database-per-service without the *bill*.

## Options considered

### Option A — One shared database, all services
Fast and cheap, but invites cross-service joins and FKs — the coupling we're explicitly avoiding.
Hard to split later. Rejected.

### Option B — One physical database instance, one schema per context, strictly enforced isolation
Logical isolation (each context only touches its schema; no cross-schema FK or join) on shared
infrastructure. Cheap now, splittable later.

### Option C — Dedicated database instance per context from day one
Maximum isolation and independent scaling/backup, maximum cost and ops at launch volume.

## Decision

We will adopt **Option B at launch**: one Postgres cluster, **one schema per bounded context**, with
the isolation rules enforced — no cross-schema foreign keys, no cross-context queries, access only
via that context's own connection/role. We move a context to its **own instance (Option C)** in
Phase 2 when its load, backup window, or availability needs diverge.

Because no code ever reaches across schemas, promoting a schema to its own instance is a connection-
string change, not a refactor.

## Consequences

- Launch on a single managed Postgres instead of 18 — large cost/ops saving.
- We must enforce "no cross-schema access" in code review and via per-context DB roles, or the
  isolation is theatre.
- Per-context migrations are still owned by that context (separate migration dirs).
- Splitting a hot context out later is mechanical (point its config at a new instance, migrate data).
