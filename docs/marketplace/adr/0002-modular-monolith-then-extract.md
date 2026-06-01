# ADR-0002: Bounded contexts — modular monolith first, extract to services later

- **Status:** Proposed
- **Date:** 2026-06-02
- **Deciders:** Tech lead (needs your sign-off)
- **Affects:** whole platform, deployment topology

## Context

The service docs define 18 services. Deploying 18 separate processes before launch maximizes
operational cost (18 pipelines, 18 datastores, distributed debugging) and risks a **distributed
monolith**: a system with a monolith's coupling *plus* network failures, if write paths fan out
synchronously across services.

The team is small and already fluent in Go/Postgres/Redis from CashPilotX. We want the *option* of
18 services without paying for it before there's a reason.

## Options considered

### Option A — Build and deploy all 18 microservices now
Maximum isolation on paper. In practice: huge ops overhead, slow local dev, high chance of a
distributed monolith, no scaling benefit at launch volume.

### Option B — One big monolith, split "someday"
Fast to start, but boundaries erode without enforced seams; "someday" never comes cleanly.

### Option C — Modular monolith per bounded context (6 deployables), with service-grade contracts from day one
Build all 18 modules as packages inside 6 context binaries. Intra-context calls are in-process and
transactional; inter-context calls are gRPC + Kafka from the start. Extract a module into its own
service only when a real signal fires (independent scaling, divergent availability, separate team).

## Decision

We will adopt **Option C**. Six deployables at launch, mapped to the bounded contexts in
`ARCHITECTURE_PRINCIPLES.md` §3: Identity & Access, Catalog, Commerce, Fulfillment, Marketplace,
Engagement. Every module owns its tables (no cross-context FK), exposes gRPC/event contracts, and
keeps its own migrations — so later extraction is mechanical, not a rewrite.

Predicted first extractions (Phase 2): `delivery` (GPS firehose + WebSocket scaling) and `search`
(Elasticsearch lifecycle).

## Consequences

- Launch with 6 pipelines and ~6 logical databases instead of 18 — far cheaper to run and develop.
- We must enforce the seams (no shared DB, no sync write chains) via review + CI, or the modular
  monolith silently fuses into a real monolith.
- Splitting later requires the contracts to already be honored — discipline now buys speed later.
- Local dev brings up one context + its dependencies, not the whole platform.
