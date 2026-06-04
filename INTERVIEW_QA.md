# CashPilotX — Senior Backend Interview Q&A

> Answers are grounded in the actual implementation.
> Stack: Go · Chi · PostgreSQL · SQLC · Redis · JWT · S3 · Zerolog · Viper
> 18 real users · Financial OS for SE Asian traders (Myanmar/Thailand)

---

## Table of Contents

1. [Project Overview Questions](#1-project-overview-questions)
2. [Architecture & Design](#2-architecture--design)
3. [Authentication & Authorization (RBAC)](#3-authentication--authorization-rbac)
4. [Database & SQLC](#4-database--sqlc)
5. [Transaction Patterns](#5-transaction-patterns)
6. [Middleware Chain](#6-middleware-chain)
7. [Redis Usage](#7-redis-usage)
8. [Idempotency](#8-idempotency)
9. [Go-Specific Patterns](#9-go-specific-patterns)
10. [API Design & Error Handling](#10-api-design--error-handling)
11. [Security](#11-security)
12. [Testing](#12-testing)
13. [Hard Questions (Senior Level)](#13-hard-questions-senior-level)

---

## 1. Project Overview Questions

---

**Q: Tell me about CashPilotX. What problem does it solve?**

CashPilotX is a financial operating system for small-to-medium traders in SE Asia — primarily Myanmar and Thailand. The core problem is debt recovery and invoice management: traders sell goods on credit, forget to follow up, and lose money. The app helps operators manage their recovery queue, generate AI-assisted follow-up messages, track customer payment behavior, and manage their team's access to that data. We currently have 18 real users on the platform.

---

**Q: Why Go for this project?**

Three reasons: First, Go's concurrency model fits the background workers — we run AI message generation, Redis Streams workers, and reminder schedulers as goroutines in the same binary without the overhead of a separate queue service. Second, SQLC gives us type-safe DB access with compile-time SQL checking, which eliminates a whole class of runtime errors for a financial app where data correctness is critical. Third, Go's single binary deployment is simple to operate on a small team.

---

**Q: What is your tech stack and why did you choose each piece?**

| Component | Choice | Reason |
|---|---|---|
| Language | Go | Concurrency, type safety, single binary |
| Router | Chi | Lightweight, idiomatic middleware chaining |
| DB | PostgreSQL | JSONB for invoice items, strong constraints, transactions |
| ORM/Queries | SQLC | Type-safe SQL, no reflection overhead, compile-time checks |
| Cache/Queue | Redis (go-redis/v9) | Rate limiting, idempotency dedup, push notification queue |
| Auth | JWT (golang-jwt/v5) | Stateless, works offline for PWA |
| Config | Viper | Multi-source config (env + yaml), hot reload ready |
| Logging | Zerolog | Zero-allocation structured logging |
| File storage | AWS S3 (aws-sdk-go-v2) | Payment proof photos, KYC documents |
| Validation | go-playground/validator | Struct tag validation on DTOs |
| Migrations | golang-migrate | Version-controlled schema changes |

---

## 2. Architecture & Design

---

**Q: Walk me through the architecture of CashPilotX.**

It's a monolith with three layers — handler, service, and DB — organized under `internal/`. The `cmd/` folder contains the entry point; `internal/handler/` holds the Chi route handlers; `internal/service/` holds business logic (one file per domain); `internal/middleware/` holds cross-cutting concerns; `db/sqlc/` holds all generated DB types and queries.

The service layer talks to the DB via SQLC-generated code and to Redis via go-redis. Background work (AI message generation, delivery workers) runs as goroutines in the same binary. HTTP is served by Chi with a middleware chain of: request ID → CORS → logging → rate limit → auth → plan gate → permission → scope → handler.

---

**Q: Why a monolith and not microservices?**

At 18 users and a two-person team, microservices would be premature. A monolith lets us share database transactions across concerns — for example, `FireReminderTx` updates the reminder, message, invoice, customer_events, and audit_log in one atomic transaction. That's impossible cleanly across services without a saga. The domain complexity is manageable in one codebase and the team doesn't need independent deployment cadences yet.

---

**Q: How is the project structured?**

```
cashpilotx/
├── cmd/              — entry point, server bootstrap
├── internal/
│   ├── api/          — route registration
│   ├── handler/      — HTTP handlers (one per domain)
│   ├── service/      — business logic
│   │   ├── auth.service.go
│   │   ├── invoice.service.go
│   │   ├── team.service.go
│   │   └── ...
│   ├── middleware/   — cors, logging, auth, plan, permission, scope, idempotency, ratelimit
│   ├── dto/          — request/response structs with validation tags
│   ├── config/       — Viper-based config
│   ├── token/        — JWT issue/verify
│   └── worker/       — background goroutines
├── db/
│   ├── migrations/   — SQL migration files (golang-migrate)
│   └── sqlc/         — generated Go types + queries + *_tx.go
└── docs/             — Swagger generated docs
```

---

## 3. Authentication & Authorization (RBAC)

---

**Q: How does authentication work?**

Login accepts an `identifier` field — if it matches a phone pattern it routes to phone+PIN (super_admin path), otherwise username+PIN (team account path). On success we issue a JWT (golang-jwt/v5) containing: `sub`, `role`, `team_id`, `org_owner_id`, `plan`, `force_pin_change`, and `login_type`. The token is short-lived; a refresh token flow handles renewal. Phone-based accounts can self-reset via OTP; username-based team accounts cannot — a superior must call `POST /api/v1/team/users/:id/reset-pin`.

---

**Q: Explain your RBAC system.**

We have four roles in a strict hierarchy: `super_admin → admin → team_leader → team_member`. The hierarchy is enforced server-side — a caller can only create roles strictly below their own. Permissions are modeled as a catalog (`permissions` table) with a `role_permissions` mapping and an override table (`user_permission_grants`) for super_admin exceptions.

Each permission has a resource, action, and qualifier — for example `invoice:view:own`, `customer:edit:team`. The middleware resolves this at request time from the JWT claims + the DB tables.

---

**Q: Walk me through the middleware chain for a protected route.**

```
Request
  → cors.go              — CORS headers
  → logging.go           — structured request log (Zerolog)
  → ratelmit.go          — Redis sliding window (per IP + per user)
  → middleware.go (auth) — verify JWT, inject claims into context
  → plan.go              — RequirePlan("pro") → 402 if plan doesn't match
  → permission.go        — RequirePermission(resource, action) → 403 if denied
  → scope.go             — ScopeContext: attaches global/team/own + user/team IDs
  → handler              — reads scope from ctx, applies to DB queries
```

Plan check runs before permission check deliberately — so a `free` plan user hitting a `pro` feature gets a `402` (upgrade prompt) not a `403` (generic error), which the frontend uses to show an upgrade nudge.

---

**Q: What is `org_owner_id` and why does it exist?**

Every team account stores the super_admin's `id` as `org_owner_id` directly on the `users` row. This lets us check plan limits with a single join — `WHERE org_owner_id = $1` — instead of walking a chain (`user → created_by → created_by → super_admin`). Chain traversal is O(depth) and error-prone; the flat `org_owner_id` is O(1) and set once at account creation.

---

**Q: What is `force_pin_change` and how is it enforced?**

When an admin creates a team account, they set an initial PIN and `force_pin_change = true`. The JWT carries this claim. The Next.js middleware intercepts every dashboard route and redirects to `/auth/change-pin` if the claim is true. The change-pin page calls `POST /api/v1/auth/change-pin`, which clears the flag and returns a fresh JWT. The user cannot navigate anywhere else until they change their PIN — the back button loops back to the change page.

---

**Q: How do you prevent data leakage between team members?**

Three layers: First, `scope.go` middleware injects the resolved scope (`global/team/own`) and the caller's `user_id`/`team_id` into the request context. Second, every DB query reads from context and appends the correct `WHERE` clause — `assigned_to = $user_id` for `team_member` on customers, for example. Third, `cost_price` is stripped at the service layer before serialization for `team_leader` and `team_member` — it never reaches the JSON output regardless of what the handler does.

---

**Q: How do you prevent a team member from seeing old invoices?**

The rule: `team_member` sees current month only. In the service layer, if `role == team_member`, we force `WHERE issued_at >= date_trunc('month', now())`. If a team_member passes `?before=` with an older date in the query string, the API returns `403`. This is enforced at the API level — UI hiding alone is not sufficient.

---

## 4. Database & SQLC

---

**Q: Why SQLC over GORM?**

SQLC generates type-safe Go from raw SQL at compile time. The benefits over GORM: (1) SQL is explicit and readable in `.sql` files — no magic, no N+1 surprises hidden by method chains; (2) compile-time type checking — if the SQL and the struct disagree, the build fails; (3) zero reflection overhead at runtime; (4) GORM's `interface{}` in many APIs is a footgun for a financial system where a wrong type cast can corrupt data.

---

**Q: How do your database migrations work?**

We use `golang-migrate` with numbered SQL files in `db/migrations/`. Each migration has an `up` and `down` file. Migrations run at startup before the server accepts requests. The schema has evolved through 26+ migrations covering: base tables, invoices, messages, payments, events, risk profiles, audit log, sync triggers, RBAC tables, products, and delivery services.

---

**Q: Describe your schema for invoices.**

Invoices have a `collection_state` enum (`pending → reminded → negotiating → promised → paid → defaulted`) that drives the recovery workflow. `items` is stored as JSONB (`[{name, qty, unit_price_minor, currency}]`) to handle variable line items without a separate table at MVP scale. Amounts are stored as `NUMERIC(14,2)` with a `CHECK (paid_amount <= amount)` constraint to prevent overpayment at the DB level. An `external_id` (`INV-2024-031`) is generated server-side in a transaction using an atomic sequence counter on the `users` table.

---

**Q: Why store money as NUMERIC and not as an integer (minor units)?**

In CashPilotX the original implementation uses `NUMERIC(14,2)` because the operators think in full currency units (Thai Baht, Myanmar Kyat). The constraint and arithmetic work correctly with NUMERIC in PostgreSQL. For the marketplace project, we switched to `BIGINT` minor units because multi-currency exchange and commission splits require integer arithmetic to avoid floating-point rounding errors — `fee + earnings` must exactly equal `gross`, which is only guaranteed with integers.

---

## 5. Transaction Patterns

---

**Q: Explain the `exec_tx.go` pattern.**

`exec_tx.go` provides a `execTx(ctx, fn)` wrapper that: begins a transaction, passes a `*Queries` scoped to that transaction to `fn`, and commits on success or rolls back on any error. Every `*_tx.go` file uses this wrapper. This means all multi-step DB operations are atomic and consistent without duplicating begin/commit/rollback logic everywhere.

```go
func (s *Store) execTx(ctx context.Context, fn func(*Queries) error) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return err }
    q := New(tx)
    if err = fn(q); err != nil {
        tx.Rollback(ctx)
        return err
    }
    return tx.Commit(ctx)
}
```

---

**Q: Walk me through `FireReminderTx`.**

This transaction fires when a reminder is delivered. In one atomic operation it:
1. Updates `reminders.status = 'sent'`, sets `sent_at`
2. Updates `messages.status = 'delivered'`
3. Updates `invoices.collection_state = 'reminded'`, increments `follow_up_count`, sets `last_contacted_at`
4. Inserts a `customer_events` row (`reminder_sent`)
5. Inserts an `operator_audit_log` row (`message_sent`)

All five writes succeed or all roll back. If a crash happens mid-way, the reminder is re-delivered on restart (idempotency key prevents double-sending to the channel API).

---

**Q: Describe `invoice_tx.go` — what does `CreateInvoiceTx` do?**

1. Atomically increments `users.invoice_sequence` with `UPDATE … RETURNING` and formats `INV-2024-031`
2. Inserts the invoice with `collection_state = 'pending'`
3. Inserts a `customer_events` row
4. Inserts an `operator_audit_log` row

The sequence increment is inside the transaction so two concurrent invoice creations can never get the same number — no race condition, no separate sequence table needed.

---

**Q: How does `move_collection_state_tx.go` work?**

It transitions an invoice's `collection_state` (e.g. `reminded → negotiating`), inserts a `customer_events` row with `{from, to, note}` in `event_value` JSONB, and writes to `operator_audit_log` — all in one transaction. The state machine logic (which transitions are valid) lives in the service layer before the transaction is called.

---

## 6. Middleware Chain

---

**Q: How does your rate limiting work?**

`ratelmit.go` (Redis sliding window) counts requests per key (IP for auth routes, `user_id` for authenticated routes) in a rolling window. If the count exceeds the limit, it returns `429` with a `Retry-After` header. Using Redis means the limit works correctly across multiple instances and survives restarts — an in-memory counter would not.

---

**Q: How does idempotency middleware work?**

`idempotency.go` reads the `Idempotency-Key` header. If a key + user_id combination is already in the `idempotency_keys` Redis key (or table), it returns the cached response with `Idempotent-Replayed: true`. If not, it wraps the response writer to capture the response body, stores it after a successful (2xx) response, and lets it through. This means a client that retries on a network error never creates a duplicate invoice or payment.

---

**Q: How does `scope.go` work?**

It reads `role`, `team_id`, and `user_id` from the JWT claims already injected by the auth middleware. Based on role it resolves scope (`global` for super_admin/admin, `team` for team_leader, `own` for team_member) and attaches all three values to the request context. Service methods read the scope from context and pass it as a filter to the SQLC query. No handler needs to manually decide scope — it's injected automatically.

---

## 7. Redis Usage

---

**Q: How many ways do you use Redis in CashPilotX?**

Four distinct uses:
1. **Rate limiting** — sliding window counters per IP/user (go-redis/v9 with INCR + EXPIRE)
2. **Idempotency key storage** — cache mutation responses with TTL (24h) for dedup
3. **Push notification queue** — web push jobs via Redis list or stream (webpush-go integration)
4. **Session/token denylist** — invalidated refresh tokens stored until expiry

---

**Q: Why Redis for idempotency keys instead of just PostgreSQL?**

Speed and TTL. An idempotency check runs on every mutation — it must be fast enough not to add visible latency. Redis lookups are sub-millisecond. PostgreSQL would work but adds a DB round-trip to every write. TTL-based expiry (24h) is native in Redis; in Postgres you'd need a cleanup cron. For CashPilotX we also keep a Postgres `idempotency_keys` table as the durable backup for crash recovery, but the hot path goes through Redis.

---

## 8. Idempotency

---

**Q: What routes require an Idempotency-Key and why?**

All mutating endpoints that a client might retry on network failure: `POST /invoices`, `POST /payments`, `POST /messages/manual`, `POST /messages/:id/approve`, `POST /reminders`, `PATCH /invoices/:id/collection-state`. GET requests are naturally idempotent. DELETE routes are idempotent by HTTP semantics. The Idempotency-Key is a UUID generated client-side; the server caches the response for 24 hours.

---

**Q: What happens if the server processes a request but the client never receives the response?**

The client retries with the same `Idempotency-Key`. The middleware finds the cached response and returns it with `Idempotent-Replayed: true` — no second DB write, no duplicate invoice. From the client's view the retry is indistinguishable from the original success. This is especially important for the PWA offline use case where a payment might be recorded while offline and synced later.

---

## 9. Go-Specific Patterns

---

**Q: How do you handle structured logging with Zerolog?**

Zerolog is zero-allocation — it builds the JSON log entry without heap allocations by using a fluent builder pattern on a stack-allocated buffer. We inject a logger into the request context in the logging middleware and read it in handlers/services. Every log entry carries `request_id`, `user_id`, `method`, `path`, and `duration`. Error logs include the full error string; info logs include the action taken.

---

**Q: How do you manage configuration?**

Viper reads from environment variables (production) and a YAML file (development). Config is parsed at startup into a typed `Config` struct in `internal/config/config.go`. This struct covers: DB pool settings, Redis, S3/MinIO, SMTP, JWT secret, rate limits, encryption key, Stripe keys, CORS origins, and feature flags. The whole app receives `Config` at startup — no `os.Getenv()` scattered through handlers.

---

**Q: How do you validate request DTOs?**

`go-playground/validator/v10` with struct tags. The validation is called in the handler before passing to the service:

```go
type CreateInvoiceRequest struct {
    CustomerID  uuid.UUID `json:"customer_id" validate:"required,uuid4"`
    Amount      int64     `json:"amount"      validate:"required,gt=0"`
    DueDate     string    `json:"due_date"    validate:"required,datetime=2006-01-02"`
}
```

The service layer can then trust the input is structurally valid and focus on business logic.

---

**Q: How do you handle errors across the stack?**

`error_service.go` defines typed error sentinels (`ErrNotFound`, `ErrForbidden`, `ErrConflict`). Handlers switch on error type to return the correct HTTP status code. This keeps HTTP concerns out of the service layer — the service returns `ErrForbidden`; the handler maps it to `403`. Unexpected errors are logged with full context (Zerolog) and return a generic `500` — internal details never leak to the client.

---

## 10. API Design & Error Handling

---

**Q: How do you design your API responses for consistency?**

All responses use a consistent envelope. Errors always return `{ "error": { "code": "not_found", "message": "..." } }`. Paginated lists return `{ "data": [...], "meta": { "page", "limit", "total" } }`. Mutations that succeed return the updated resource. The Swagger docs (swaggo/swag) are generated from handler annotations and kept in sync.

---

**Q: Explain the sync endpoint for the PWA.**

`GET /api/v1/sync?since=<timestamp>` returns only rows changed after the cursor — invoices, customers, and the recovery queue. Each table has an `updated_at` column maintained by a Postgres trigger (`set_updated_at()`). The server time is returned as the next cursor — always server time, never client clock, to avoid clock skew issues. Offline PWA operations write to a `pending_ops` queue locally and replay on reconnect using idempotency keys.

---

## 11. Security

---

**Q: How do you store PINs?**

With `bcrypt` (`golang.org/x/crypto/bcrypt`). PINs are hashed before storage; the plaintext is never persisted. On login, we compare the submitted PIN against the stored hash with `bcrypt.CompareHashAndPassword`. The cost factor is set in configuration — tunable without code changes.

---

**Q: How do you protect cost_price from leaking?**

Three separate defenses: (1) `permission.go` middleware rejects any request from `team_leader`/`team_member` to the products management endpoints. (2) `product.service.go` strips `cost_price` from the response struct when the caller's role is `team_leader` or `team_member` before serialization. (3) The SQLC query for the team-facing product list can optionally omit the column. The first two are implemented; defense in depth means even if one layer fails, the others hold.

---

**Q: How do you prevent SQL injection?**

SQLC generates parameterized queries only — there is no string interpolation of user input into SQL anywhere in the generated code. Every query uses `$1`, `$2` placeholders with the pgx driver. This makes SQL injection structurally impossible in SQLC-generated code; raw SQL is only used in migrations which are not user-controlled.

---

**Q: How do you handle file uploads securely?**

Files (payment proof photos, KYC docs) go to S3 via `aws-sdk-go-v2`. We validate MIME type with `gabriel-vasile/mimetype` — checking the actual file bytes, not just the extension, since an attacker can rename a `.exe` to `.jpg`. Files are stored under a UUID key (not the original filename). Private documents use pre-signed URLs with short TTLs; public assets use CDN URLs.

---

## 12. Testing

---

**Q: How is the project tested?**

Integration tests in `db/sqlc/*_test.go` use `miniredis/v2` (in-memory Redis) and a real PostgreSQL instance (testcontainers pattern via environment variable). The `main_test.go` sets up a test DB and runs all SQLC query tests against it — including the transaction functions. Service layer tests use the real DB layer; there are no mocks of the DB (mocks don't replicate transaction semantics, constraints, or `FOR UPDATE` locking).

---

**Q: Why testify over the standard `testing` package?**

`testify/require` gives `require.NoError(t, err)` which stops the test immediately on failure — critical in setup code where subsequent assertions would be meaningless. `assert` continues on failure. The diff output for struct comparison failures is much more readable than `t.Errorf`. For a financial app where test failure messages matter, readable output reduces debug time.

---

## 13. Hard Questions (Senior Level)

---

**Q: What happens if the server crashes between writing to the DB and sending the AI-generated message to the customer?**

The Redis Stream (`fl:deliver`) entry was added before or after the DB write depending on the flow. If the stream entry was added but the server crashed before XACK, Redis will redeliver the message to the worker on restart (at-least-once semantics). The `idempotency_key` stored in the stream payload prevents double-sending to the channel API — the channel is called with the same idempotency key, and the provider (LINE/Messenger) deduplicates on their end. The `messages.status` is updated only after successful delivery, so the UI correctly reflects the retry.

---

**Q: How do you handle concurrent invoice creation to prevent duplicate sequence numbers (INV-2024-001)?**

`CreateInvoiceTx` does `UPDATE users SET invoice_sequence = invoice_sequence + 1 RETURNING invoice_sequence` inside a transaction. PostgreSQL's `UPDATE` takes a row-level lock on the `users` row for the duration of the transaction. Two concurrent calls will serialize — one waits for the other to commit. The lock is held only for the duration of the transaction (milliseconds), so throughput is high even under concurrency. We never generate the sequence outside a transaction.

---

**Q: A team_member claims they can see another member's customers through the API. How do you investigate and fix it?**

1. Check `scope.go` — is `ScopeContext` correctly resolving `own` for `team_member`?
2. Check the SQLC query for customers — is it correctly using the scope filter from context?
3. Check whether the handler is reading scope from context or from query params (a client could pass `?user_id=other_user`).
4. Check the service layer — is `assigned_to = $user_id` correctly applied?
5. Check if any BFF or aggregation endpoint bypasses the normal handler.
6. Write a failing test: two team_members, assert that A cannot see B's customers.
7. Fix at the service layer (not just the handler — service is the last defense before the DB).

The root cause is almost always the scope filter missing from one query path.

---

**Q: How would you scale CashPilotX if it grew from 18 to 18,000 users?**

Phased approach:
1. **DB first:** Add read replicas for all read-heavy endpoints (recovery queue, dashboard, customer list). The write path stays on primary.
2. **Connection pooling:** PgBouncer in front of PostgreSQL — Go's pgx already supports it.
3. **Redis cluster:** Current single Redis → Redis Cluster for the rate limiter and idempotency cache.
4. **Worker separation:** Extract the AI generation and delivery workers from the main binary into separate deployables so they can scale independently of the API.
5. **Caching layer:** Recovery queue scores don't change every second — add a 30-second cache on the expensive risk-score computation.
6. **Read model:** If the recovery queue sorting becomes slow, precompute and materialize the ranked list on `updated_at` trigger events.
The schema was designed with proper indexes from day one (`idx_invoices_state`, `idx_customers_assigned`, etc.) so most queries scale well without schema changes.

---

**Q: The product requirement says "cost_price must never reach team_member." How many places do you enforce this and why?**

Three places:
1. **Route-level (permission middleware):** `team_member` cannot reach the product management endpoints at all.
2. **Service layer:** `product.service.go` strips `cost_price` from the response struct before returning — regardless of which handler calls it.
3. **Potential SQLC query:** A separate query that omits the column for the team-facing read path.

Why three? Defense in depth. If a developer adds a new handler and forgets the permission middleware, the service layer still strips it. If they accidentally call the wrong service method, the query might not even fetch it. Security rules for sensitive financial data should survive a developer mistake in any single layer.

---

**Q: Why does `plan.go` return 402 instead of 403?**

Semantic correctness and UX. `403 Forbidden` means "you are authenticated but not authorized" — implying a permanent denial. `402 Payment Required` means "this resource requires payment." The frontend distinguishes: a `403` shows a generic error; a `402` triggers the upgrade prompt component. This is a deliberate UX decision — a free-plan user hitting a pro feature should see "upgrade to access this" not "you don't have permission." The middleware returns `{ "upgrade": true }` in the body so the frontend knows exactly what to render.

---

*CashPilotX — production Go backend · 18 real users · SE Asian financial OS*
*Prepared for senior backend interviews · answers reflect actual implementation*
