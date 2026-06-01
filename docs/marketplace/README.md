# CashPilotX Marketplace — Architecture Docs

> Codename: `bazaar` · Multi-vendor e-commerce marketplace for SE Asia
> Companion to **CashPilotX** (the financial/admin OS) — orders here feed invoices there.
> Backend: Go · PostgreSQL (per service) · Kafka · RabbitMQ · Redis · gRPC · WebSocket/SSE

These docs follow the same style as `cashpilotx_v5_arch_final.md`: real schema,
event flows, API route tables, transaction functions, sprint plans, and inline
`⚠ GAP` markers for unresolved decisions.

---

## How to read these docs

1. **Start with [`ARCHITECTURE_PRINCIPLES.md`](ARCHITECTURE_PRINCIPLES.md)** — the foundation: bounded contexts, the modular-monolith→services evolution path, the innovation budget, core patterns, cross-cutting pillars, and the non-negotiable rules. This is the "how we build it safely" layer — read it first.
2. Then [`00_system_architecture.md`](00_system_architecture.md) — the whole platform: service map, Kafka topology, tech stack, money flow, sprint plan, and cross-service open decisions (`⚠ GAP 1–6`).
3. Skim [`adr/`](adr/README.md) — the decision records (why, not just what). Several are **Proposed** and need your sign-off.
4. Read the **cross-cutting docs** — platform-wide concerns that apply to every service:
   - [`CROSS_CUTTING_AUTHZ.md`](CROSS_CUTTING_AUTHZ.md) — authentication & authorization model
   - [`EVENT_CATALOG.md`](EVENT_CATALOG.md) — the source of truth for every Kafka event + governance
   - [`TESTING_STRATEGY.md`](TESTING_STRATEGY.md) — how we keep a distributed system trustworthy
5. Then individual service docs as needed. Each is self-contained: purpose, DB schema, Kafka topics, API routes, gRPC, transaction functions, and per-service gaps.

> **Mental model:** `ARCHITECTURE_PRINCIPLES` = the rules · `adr/` = the decisions · cross-cutting docs = platform-wide concerns · `00_…` = the target system · `01–18` = the modules. The 18 services are **modules inside 6 bounded contexts**, deployed as ~6 context-services first and extracted to full services only when scale or team demands (see ADR-0002).

---

## Service index

| # | Doc | Service | One-liner |
|---|---|---|---|
| 00 | [00_system_architecture.md](00_system_architecture.md) | **Master** | Whole-platform view, Kafka backbone, sprint plan |
| 01 | [01_api_gateway.md](01_api_gateway.md) | api-gateway | Edge routing, auth, rate limit, BFF |
| 02 | [02_auth_service.md](02_auth_service.md) | auth-service | PASETO tokens, OAuth2, refresh rotation |
| 03 | [03_user_service.md](03_user_service.md) | user-service | Customer profiles, addresses |
| 04 | [04_product_service.md](04_product_service.md) | product-service | Catalog, categories, variants, photos |
| 05 | [05_inventory_service.md](05_inventory_service.md) | inventory-service | Stock, reservations, warehouse sync |
| 06 | [06_cart_service.md](06_cart_service.md) | cart-service | Multi-seller cart, Redis-backed |
| 07 | [07_order_service.md](07_order_service.md) | order-service | Parent/sub-order state machine |
| 08 | [08_payment_service.md](08_payment_service.md) | payment-service | Collect, split, refund |
| 09 | [09_delivery_service.md](09_delivery_service.md) | delivery-service | Tracking timeline, GPS route, live map |
| 10 | [10_notification_service.md](10_notification_service.md) | notification-service | Email/SMS/push, consumes everything |
| 11 | [11_search_service.md](11_search_service.md) | search-service | Elasticsearch product search |
| 12 | [12_file_service.md](12_file_service.md) | file-service | Upload, image processing, CDN signing |
| 13 | [13_review_service.md](13_review_service.md) | review-service | Product + seller reviews |
| 14 | [14_chat_service.md](14_chat_service.md) | chat-service | Customer ↔ seller realtime chat |
| 15 | [15_seller_service.md](15_seller_service.md) | seller-service | Seller onboarding, KYC, store |
| 16 | [16_commission_service.md](16_commission_service.md) | commission-service | Platform fee calculation |
| 17 | [17_payout_service.md](17_payout_service.md) | payout-service | Seller payouts after hold period |
| 18 | [18_storefront_service.md](18_storefront_service.md) | storefront-service | Per-seller store pages |

---

## Conventions used across all docs

- **Language:** Go · **Router:** Chi · **DB queries:** SQLC · **DB:** PostgreSQL (database-per-service)
- **Events:** Kafka (`franz-go`), past-tense topic names (`order.created`), partitioned by aggregate id
- **Task queues:** RabbitMQ (one-time jobs, delays, retries, DLQ)
- **Cache / session / presence:** Redis
- **Realtime:** WebSocket (chat, live delivery map) + SSE (order status bar)
- **Internal RPC:** gRPC (typed service-to-service) · **External API:** REST via gateway
- **Auth:** PASETO v4 (short-lived) + rotating refresh token
- **Tx naming:** `{Verb}{Entity}Tx` (e.g. `CreateOrderTx`)
- **IDs:** UUID v7 (time-sortable) primary keys
- **Money:** stored in minor units (`INT`/`BIGINT`), never floats
