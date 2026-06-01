# ADR-0001: Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-06-02
- **Deciders:** Tech lead
- **Affects:** whole platform

## Context

This marketplace spans many contexts and will outlive the memory of the decisions that shaped it.
The service docs describe *what* the system is; they don't preserve *why* we chose orchestration over
choreography, or why RabbitMQ is deferred. Without a record, every past decision gets re-litigated
and new engineers can't tell a deliberate choice from an accident.

## Options considered

### Option A — Document decisions in the service docs / wiki prose
Decisions get buried in long documents, edited in place, and lose their history.

### Option B — Lightweight ADRs (Michael Nygard format), one file per decision, in the repo
Versioned with the code, short, immutable once accepted, easy to supersede.

## Decision

We will keep ADRs in `docs/marketplace/adr/`, one decision per file, using `template.md`. Accepted
ADRs are immutable; a changed decision gets a new ADR that supersedes the old one. Any choice that is
hard to reverse or crosses a context boundary requires an ADR.

## Consequences

- The "why" is preserved next to the code and reviewed in PRs.
- Small ongoing discipline cost (write the ADR before merging the change).
- The master doc's `⚠ GAP 1–6` decisions will each be resolved as an ADR.
