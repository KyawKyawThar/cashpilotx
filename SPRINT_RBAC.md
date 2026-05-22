# Sprint Plan — RBAC & Role Management
**Project:** CashPilotX — Thai/Chinese → Myanmar Distribution  
**Feature:** Auth Refactor + Plan Gating + Role-Based Access Control + Team Management + Product Catalog  
**Total Sprints:** 7 (0, A → F)

---

## Role Hierarchy (reference)

```
CashPilotX Platform
        │
        └── Super Admin  (one per company)
                │
        ┌───────┼───────┐
        ▼       ▼       ▼
      Admin  Team Lead  Team Member
        │       │
    ┌───┴──┐    └──▶ Team Member
    ▼      ▼
Team    Team
Lead   Member
```

## Plan Gating (reference)

| Plan | RBAC Access | How to upgrade (MVP) |
|---|---|---|
| `free` | Solo user only — no teams, no roles | Manual `UPDATE users SET plan = 'pro'` in DB |
| `pro` | Full RBAC — teams, roles, products catalog | Manual DB update for now, payment flow later |
| `enterprise` | Reserved for future | — |

> For MVP: no payment integration needed. DM the owner, flip the DB row, done.  
> When billing is built later, the upgrade path just automates that one `UPDATE`.

---

## Auth Identity (reference)

| Role | Login with | Who sets credentials | PIN reset |
|---|---|---|---|
| super_admin | Phone + PIN | Self (registration) | OTP to phone |
| admin | Username + PIN | Super Admin creates | Superior resets |
| team_leader | Username + PIN | Admin creates | Superior resets |
| team_member | Username + PIN | Team Leader creates | Superior resets |

> Phone number is stored as a **contact field** for team accounts, not a login identifier.
> If a team member leaves, admin deactivates the account and creates a new one. No personal phone dependency.

## Data Scope (reference)

| Role | Customers | Invoices | Payments |
|---|---|---|---|
| super_admin | all | all | all |
| admin | all | all | all |
| team_leader | own team | own team | own team |
| team_member | own accounts | current month only | own only |

---

## Sprint 0 — Auth Refactor
**Goal:** Decouple team account identity from personal phone numbers before building anything else.  
**Why first:** Every sprint after this depends on the new login flow and JWT shape.

### Backend (Go)

- [ ] **Migration 000022** — username + force_pin_change + phone optional
  ```sql
  ALTER TABLE users
    ADD COLUMN username          TEXT UNIQUE,
    ADD COLUMN force_pin_change  BOOLEAN NOT NULL DEFAULT false,
    ALTER COLUMN phone           DROP NOT NULL,
    ADD CONSTRAINT chk_user_identity
      CHECK (phone IS NOT NULL OR username IS NOT NULL);
  ```
  - Existing super_admin rows: phone stays, username stays NULL — valid
  - New team accounts: username set, phone optional contact info

- [ ] **Update login service** — `internal/service/auth.service.go`
  - Change `LoginRequest` from `{ phone, pin }` to `{ identifier, pin }`
  - If identifier matches phone pattern (`^\+?[0-9]{8,15}$`) → phone lookup (super_admin)
  - Otherwise → username lookup (team accounts)
  - Single endpoint handles both — no breaking change to URL

- [ ] **Update `POST /api/v1/auth/login` handler**
  - Accept `{ identifier, pin }` instead of `{ phone, pin }`
  - Return same token shape — no frontend token parsing changes

- [ ] **New endpoint: `POST /api/v1/auth/change-pin`**
  - Body: `{ current_pin, new_pin, confirm_pin }`
  - Clears `force_pin_change = false` on success
  - Returns fresh JWT with `force_pin_change: false`

- [ ] **Update JWT claims**
  - Add `force_pin_change bool`
  - Add `login_type: "phone" | "username"`
  - Add `plan: "free" | "pro" | "enterprise"`
  - Existing: `role`, `team_id` (added Sprint A — plan now, wire in Sprint A)

- [ ] **Update forgot-pin flow** — `POST /api/v1/auth/forgot-pin`
  - Detect account type from `identifier`
  - Phone → existing OTP flow (unchanged)
  - Username → return `403` with message: `"Contact your administrator to reset your PIN"`

### Frontend (Next.js)

- [ ] **Update login page** — `src/app/(auth)/auth/login/auth-client.tsx`
  - Add toggle at top of phone entry step: `Phone` | `Username`
  - **Phone tab** (super_admin): existing numpad flow — no changes
  - **Username tab** (team accounts):
    - Text input for username (keyboard, not numpad)
    - Then PIN numpad (same 6-digit UI)
  - Store selected login type in local state, pass `identifier` to server action

- [ ] **New page: `/auth/change-pin`**
  - Triggered automatically when JWT contains `force_pin_change: true`
  - UI: current PIN numpad → new PIN numpad → confirm PIN numpad
  - On success: refresh session, redirect to dashboard
  - Cannot be skipped — back button loops back to this page

- [ ] **Update middleware** — `src/middleware.ts`
  - After auth check: if session has `force_pin_change: true` → redirect to `/auth/change-pin`
  - Add `/auth/change-pin` to the public (unprotected) route list

- [ ] **Update forgot-pin page** — `src/app/(auth)/auth/forgot-pin/`
  - If user typed a username (not a phone number) → replace OTP UI with:
    > "Team accounts cannot self-reset. Ask your Admin or Team Leader to reset your PIN."
  - Phone input → existing OTP flow unchanged

### Deliverable
- Super admin logs in with phone + PIN (unchanged experience)
- Team accounts log in with username + PIN
- First login forces PIN change before reaching dashboard
- Forgot PIN for team accounts shows "contact your admin" — no OTP sent

---

## Sprint A — Database Foundation
**Goal:** All new tables and schema changes in place. No app logic yet.

### Backend (Go migrations)

- [ ] **Migration 000023** — teams + role + org ownership on users
  - `CREATE TABLE teams (id, name, admin_id, created_at)`
  - `CREATE TYPE user_role AS ENUM ('super_admin','admin','team_leader','team_member')`
  - `ALTER TABLE users ADD COLUMN role user_role NOT NULL DEFAULT 'super_admin'`
  - `ALTER TABLE users ADD COLUMN team_id UUID REFERENCES teams(id)`
  - `ALTER TABLE users ADD COLUMN created_by UUID REFERENCES users(id)`
  - `ALTER TABLE users ADD COLUMN org_owner_id UUID REFERENCES users(id)`
    - `super_admin`: `org_owner_id = their own id`
    - All team accounts: `org_owner_id = super_admin's id`
    - Plan check: one join — no chain traversal needed
  - **Update JWT** to include `role`, `team_id`, `plan`, `org_owner_id` in access token claims

- [ ] **Migration 000024** — customer account ownership
  - `ALTER TABLE customers ADD COLUMN assigned_to UUID REFERENCES users(id)`
  - `ALTER TABLE customers ADD COLUMN credit_limit_approved_by UUID REFERENCES users(id)`
  - `CREATE INDEX idx_customers_assigned ON customers(assigned_to)`

- [ ] **Migration 000025** — products table
  - `id, name, sku, origin (thai/chinese/other), category, unit`
  - `cost_price BIGINT` — never exposed to team_leader or team_member
  - `selling_price BIGINT, currency CHAR(3)`
  - `is_active BOOLEAN NOT NULL DEFAULT true`

- [ ] **Migration 000025b** — delivery services catalog
  - `CREATE TABLE delivery_services (id, name, price BIGINT, currency CHAR(3), is_active BOOLEAN NOT NULL DEFAULT true, created_by UUID REFERENCES users(id), created_at)`
  - Admin-managed, same pattern as products
  - Used as a picker in invoice creation — no free-text entry

- [ ] **Migration 000026** — permissions catalog
  - `CREATE TYPE data_scope AS ENUM ('global','team','own')`
  - `CREATE TABLE permissions (id, resource, action, qualifier, description)`
  - `CREATE TABLE role_permissions (role, permission_id, scope)`
  - `CREATE TABLE user_permission_grants (id, user_id, permission_id, scope, scope_team_id, granted_by, reason, expires_at)`

- [ ] **Seed SQL** — populate `permissions` + `role_permissions` with full matrix
  - invoice: view, view_history, create, edit, delete, export
  - customer: view, create, edit, delete, view_risk_profile, set_credit_limit
  - payment: view, record, void
  - reminder: view, create, cancel
  - message: view, send
  - audit_log: view, revert
  - report: view, export
  - recovery_queue: view, action
  - product: view_selling, view_cost, manage
  - channel_config: view, manage
  - team: view, create_admin, create_team_leader, create_team_member, edit, delete, grant_permission

### Deliverable
All migrations pass `migrate up`. Seed runs without error. JWT contains `role`, `team_id`, `force_pin_change`, `login_type`.

---

## Sprint B — Go Backend: Permission Middleware
**Goal:** Every API route checks role and scopes DB queries automatically.

### Backend (Go)

- [ ] **`internal/middleware/plan.go`** — plan gate (runs before permission check)
  - `RequirePlan("pro")` middleware factory
  - Reads `plan` from JWT claims
  - Returns `402 Payment Required` with `{ "upgrade": true }` if plan doesn't match
  - Wraps entire `/api/v1/team/*` and `/api/v1/products/*` route groups

- [ ] **`internal/middleware/permission.go`**
  - `RequirePermission(resource, action, qualifier string)` — middleware factory
  - Reads `role` + `team_id` from JWT claims
  - Checks `role_permissions` first, then `user_permission_grants` for super_admin exceptions
  - Returns `403` if not allowed

- [ ] **`internal/middleware/scope.go`**
  - `ScopeContext` — attaches resolved scope (`global/team/own`) + `team_id` + `user_id` to request context
  - Helper: `ApplyScope(query, scope, userID, teamID)` — appends correct WHERE clause

- [ ] **Update existing route handlers** to use scope from context
  - `GET /api/v1/invoices` — apply scope + date filter for `team_member`
  - `GET /api/v1/customers` — apply `assigned_to` filter for `team_member`
  - `GET /api/v1/payments` — apply scope
  - `GET /api/v1/reminders` — apply scope
  - `GET /api/v1/audit-log` — apply scope
  - `GET /api/v1/sync` — apply scope to both invoices + customers

- [ ] **Cost price protection**
  - Strip `cost_price` at service layer (not handler) when `role` is `team_leader` or `team_member`

- [ ] **Invoice history gate**
  - If `role == team_member` → force `WHERE issued_at >= date_trunc('month', now())`
  - Return `403` if team_member explicitly passes `?before=` older than current month

### Deliverable
All existing endpoints respect role scope. Postman tests pass for each role.

---

## Sprint C — Go Backend: Team & User Management API
**Goal:** Super Admin and Admin can create and manage accounts below them.

### Backend (Go)

- [ ] **DTOs** — `internal/dto/team_dto.go`, `internal/dto/user_mgmt_dto.go`
  ```go
  CreateUserRequest {
    name      string
    username  string   // company-assigned, not personal phone
    phone     string?  // optional contact info only
    pin       string   // initial PIN (admin sets, user forced to change)
    role      user_role
    team_id   uuid?
  }
  TeamMemberResponse {
    id, name, username, phone?, role, team_id, created_by, force_pin_change, created_at
  }
  ```

- [ ] **Service** — `internal/service/team.service.go`
  - `CreateUser(ctx, req)`:
    - Validates caller can only create roles **strictly below** their own
    - Sets `force_pin_change = true` automatically
    - Hashes initial PIN
  - `ListTeamMembers(ctx, callerRole, teamID)` — scoped list
  - `UpdateMember(ctx, id, req)` — edit name, username, phone, team_id
  - `DeactivateMember(ctx, id)` — soft delete (`is_active = false`)
  - `ResetMemberPin(ctx, callerID, memberID, tempPin)`:
    - Verifies caller is superior to member
    - Sets new hashed PIN + `force_pin_change = true`

- [ ] **Handlers + Routes**
  - `POST   /api/v1/team/users` — create account (username-based)
  - `GET    /api/v1/team/users` — list members (scoped by role)
  - `PATCH  /api/v1/team/users/:id` — edit member
  - `DELETE /api/v1/team/users/:id` — deactivate member
  - `POST   /api/v1/team/users/:id/reset-pin` — superior resets member PIN
  - `GET    /api/v1/teams` — list teams (admin+ only)
  - `POST   /api/v1/teams` — create team (admin+ only)

- [ ] **Permission grants API** (super_admin only)
  - `POST   /api/v1/team/users/:id/grants` — grant exception permission
  - `DELETE /api/v1/team/users/:id/grants/:grant_id` — revoke grant

### Deliverable
Full user lifecycle API. Username-based accounts only. Role boundary enforced server-side. PIN reset by superior, not OTP.

---

## Sprint D — Go Backend: Products API
**Goal:** Product catalog with cost price protection by role.

### Backend (Go)

- [ ] **DTOs** — `internal/dto/product_dto.go`
  - `ProductResponse` — includes `cost_price` only when `role` is `admin` or `super_admin`
  - `CreateProductRequest { name, sku, origin, category, unit, cost_price, selling_price, currency }`

- [ ] **SQLC queries** — `db/query/products.sql`
  - `ListProducts(isActive)` — returns all fields (service strips cost_price)
  - `GetProduct(id)`
  - `CreateProduct`, `UpdateProduct`, `DeactivateProduct`

- [ ] **Service** — `internal/service/product.service.go`
  - `ListProducts(ctx, callerRole)` — strips `cost_price` for team_leader/team_member
  - `CreateProduct`, `UpdateProduct` — admin+ only (permission middleware guards route)

- [ ] **Handlers + Routes**
  - `GET    /api/v1/products` — list (cost_price hidden by role)
  - `POST   /api/v1/products` — create (admin+ only)
  - `PATCH  /api/v1/products/:id` — edit (admin+ only)
  - `DELETE /api/v1/products/:id` — deactivate (admin+ only)

- [ ] **Delivery Services API** — `internal/service/delivery_service.service.go`
  - `GET    /api/v1/delivery-services` — list active services (all roles)
  - `POST   /api/v1/delivery-services` — create (admin+ only)
  - `PATCH  /api/v1/delivery-services/:id` — edit (admin+ only)
  - `DELETE /api/v1/delivery-services/:id` — deactivate (admin+ only)
  - DTOs: `CreateDeliveryServiceRequest { name, price, currency }`, `DeliveryServiceResponse`

- [ ] **Link products to invoice items** — catalog-enforced (no free text)
  - Invoice `items` JSONB already stores `{name, unit_price}` — add `product_id` (required when product exists)
  - Add `delivery_service_id UUID REFERENCES delivery_services(id)` to invoices table
  - Frontend must pick from catalog; backend rejects invoice if `product_id` references a non-existent or inactive product
  - `selling_price` pre-fills from product and is **not editable** by team_member

### Deliverable
Product and Delivery Service APIs live. All catalog fields are picker-only — no free-text entry. Cost price never leaks to team_member or team_leader.

---

## Sprint E — Frontend: Permission Context + Guards
**Goal:** Frontend knows the user's role and hides/shows features accordingly.

### Frontend (Next.js)

- [ ] **Update session** — `src/lib/auth.ts`
  - Include `role`, `team_id`, `plan`, `org_owner_id`, `force_pin_change`, `login_type` in `getSession()` return type
  - Decode from JWT claims

- [ ] **`src/hooks/use-permission.ts`**
  ```ts
  usePermission(resource: string, action: string, qualifier?: string) → boolean
  ```
  - Reads `role` from session
  - Matches against a client-side permission map (mirrors `role_permissions` seed data)

- [ ] **`src/components/can.tsx`**
  ```tsx
  <Can do="edit" on="invoice">
    <EditButton />
  </Can>
  // with fallback for disabled state:
  <Can do="void" on="payment" fallback={<DisabledButton />}>
    <VoidButton />
  </Can>
  ```

- [ ] **Route-level guards** — update `src/middleware.ts`
  - `/reports` → redirect `team_member` to dashboard
  - `/recovery-queue` → redirect `team_member` to dashboard
  - `/settings/channels` → redirect `team_leader` + `team_member` to dashboard
  - `/auth/change-pin` → redirect away if `force_pin_change` is false
  - `/settings/team` → redirect `free` plan users to upgrade page
  - `/products` → redirect `free` plan users to upgrade page

- [ ] **`src/components/role-badge.tsx`**
  - Small pill: `SUPER ADMIN` / `ADMIN` / `TEAM LEAD` / `SALES REP`
  - Show in sidebar footer or profile drawer

- [ ] **`src/components/upgrade-prompt.tsx`**
  - Shown to `free` plan users who hit a `pro` feature
  - Simple card: "This feature requires Pro plan. Contact us to upgrade."
  - No payment flow needed for MVP — just a contact nudge

### Deliverable
Any page or element wrapped in `<Can>` correctly hides for unauthorized roles.

---

## Sprint F — Frontend: Team Management + Products Pages
**Goal:** Full UI for creating accounts, managing the team, and the product catalog.

### Frontend (Next.js)

#### Team Management — `/settings/team`

- [ ] **Page + client component**
  - Member list: name, username, role badge, team, status (active/inactive), created date
  - Filter tabs: All / Admin / Team Lead / Sales Rep
  - **`+ Add Member` bottom sheet:**
    - Name, username (company-assigned, e.g. `kyaw_yangon`)
    - Optional phone (contact only — label it clearly)
    - Initial PIN (admin sets) + "Force PIN change on first login" toggle (default ON)
    - Role selector — only shows roles below current user's role
    - Team assignment dropdown
  - **Edit member sheet:** name, username, phone, team reassignment
  - **Reset PIN sheet:** enter new temporary PIN → member forced to change on next login
  - Deactivate with confirm overlay ("This user will lose all access immediately")

- [ ] **Permission exception panel** — visible to `super_admin` only
  - Per-member: list active grants with expiry + reason
  - Grant form: resource + action + qualifier, optional expiry date, reason text
  - Revoke button per grant row

#### Products — `/products`

- [ ] **Page + client component**
  - Product list: name, SKU, origin badge (Thai / Chinese), category, selling price
  - `Cost Price` column — only rendered when `role` is `admin` or `super_admin`
  - Filter bar: origin / category / active toggle
  - **`+ Add Product` sheet** (admin+ only):
    - Name, SKU, origin (Thai/Chinese/Other), category, unit
    - Cost price (admin+ only field)
    - Selling price
    - Currency selector
  - Edit / Deactivate actions (admin+ only, guarded with `<Can>`)

#### Updates to existing pages

- [ ] **Invoices page**
  - Hide history filter / date range for `team_member`
  - Hide Edit + Cancel buttons for `team_member`
  - Hide Export button for `team_member`
  - **Invoice line items** — replace free-text product name with a **product picker** (searchable dropdown, active products only); `selling_price` auto-fills and is locked for `team_member`
  - **Delivery service** — replace free-text/note field with a **delivery service picker** (dropdown from catalog); admin must add new services before they appear here

- [ ] **Customers page**
  - Show `Assigned To` rep name on each customer card (visible to team_leader+)
  - **Assigned To field** (new/edit customer form) — replace free-text with a **team member picker** (select from active users scoped to current org); `team_member` cannot change their own assignment
  - Hide risk profile card for `team_member`
  - Hide credit limit field for `team_leader` + `team_member`
  - Hide Delete / Block action for `team_member`

- [ ] **Payments page**
  - Hide Void action for `team_member`
  - Hide team summary stats strip for `team_member`

- [ ] **Activity Log page**
  - Hide Revert button for `team_member` + `team_leader`

- [ ] **Sidebar / nav**
  - Hide Reports link for `team_member`
  - Hide Recovery Queue link for `team_member`
  - Hide Channel Settings for `team_leader` + `team_member`

### Deliverable
Full working UI. Each role sees exactly what the permission matrix allows.

---

## Summary

| Sprint | Focus | Output |
|---|---|---|
| **0** | Auth refactor — username login + force PIN change + plan in JWT | Company-owned credentials, no personal phone dependency |
| **A** | DB migrations + seed + JWT (incl. org_owner_id, plan) | Schema ready |
| **B** | Plan gate middleware + permission middleware + scope enforcement | All APIs are plan-aware and role-aware |
| **C** | Team & user management API | Account creation lifecycle |
| **D** | Products API | Catalog with cost price protection |
| **E** | Frontend permission context + plan guards + upgrade prompt | Role-aware and plan-aware UI framework |
| **F** | Team mgmt page + products page + UI updates | Full feature complete |

---

## Key rules (never UI-only — always enforced at API level)

1. **Invoice history** — API enforces `issued_at >= date_trunc('month', now())` for `team_member`
2. **Cost price** — stripped at service layer before JSON serialization, never reaches team_member/team_leader
3. **Customer scope** — `WHERE assigned_to = $user_id` for `team_member`
4. **Role creation boundary** — server rejects creating a role equal to or above caller's own role
5. **Permission grants** — only `super_admin` can write to `user_permission_grants`
6. **PIN reset** — team accounts have no self-serve OTP reset; only a superior can call `reset-pin`
7. **Force PIN change** — JWT with `force_pin_change: true` cannot access any dashboard route until PIN is changed
8. **Plan gate** — `RequirePlan("pro")` runs before permission check; returns `402` not `403` so frontend can show upgrade prompt instead of generic error
9. **org_owner_id** — every team account stores the super_admin's id directly; plan check is a single join, not a chain traversal
10. **Catalog-enforced entry** — products, delivery services, and assigned persons are always selected from a predefined list; free-text entry is not allowed. Admin must add new catalog items before they can be used in invoices or customer forms. Backend validates that referenced IDs are active and belong to the same org.
