# Cross-Cutting — Authentication & Authorization

> How identity, roles, and permissions work across every context. Implements
> `ARCHITECTURE_PRINCIPLES.md` §7.1. AuthN = "who are you" (auth-service + gateway).
> AuthZ = "what may you do" (coarse at the edge, fine-grained in the owning context).

---

## Table of Contents

1. [Two Layers: Coarse Edge, Fine-Grained Core](#1-two-layers-coarse-edge-fine-grained-core)
2. [Identity & Token Model](#2-identity--token-model)
3. [Roles](#3-roles)
4. [Permission Model (RBAC now, ABAC later)](#4-permission-model-rbac-now-abac-later)
5. [Resource Ownership — the part most systems get wrong](#5-resource-ownership--the-part-most-systems-get-wrong)
6. [Permission Matrix](#6-permission-matrix)
7. [Service-to-Service Auth](#7-service-to-service-auth)
8. [Special Cases: Driver, Admin, Impersonation, Seller Staff](#8-special-cases-driver-admin-impersonation-seller-staff)
9. [Multi-Tenant Isolation (sellers)](#9-multi-tenant-isolation-sellers)
10. [Rules & Anti-Patterns](#10-rules--anti-patterns)

---

## 1. Two Layers: Coarse Edge, Fine-Grained Core

Authorization is **never** done in only one place.

```
┌─────────────────────────────────────────────────────────────┐
│ API GATEWAY  — COARSE authz                                   │
│  • Is the token valid? (PASETO verify, local)                 │
│  • Does the route's required role match the token's role?     │
│  • Rate limit / abuse                                         │
│  → Rejects the obviously-unauthorized (a customer hitting     │
│    /admin/*) before it ever reaches a service.                │
└───────────────────────────┬───────────────────────────────────┘
                            │ trusted claims forwarded (sub, role, seller_id)
┌───────────────────────────▼───────────────────────────────────┐
│ OWNING SERVICE — FINE-GRAINED authz                            │
│  • Does THIS user own THIS resource?                          │
│    (does seller A own product X? is this the buyer's order?)  │
│  • Business-rule gates (can't review unless delivered)        │
│  → The only place that can answer ownership, because it owns  │
│    the data.                                                  │
└───────────────────────────────────────────────────────────────┘
```

**Why both:** the gateway can't know whether order `123` belongs to the caller — only order-service
can. The gateway stops broad classes of abuse cheaply; the service enforces the truth.

---

## 2. Identity & Token Model

(See `02_auth_service.md` for issuance detail.)

- **Access token:** PASETO v4 public (Ed25519), 15 min. Verified **locally** by the gateway with the
  public key — no network call per request.
- **Refresh token:** opaque, 30 d, one-time-use, family-based reuse detection.

### Claims (the trusted context every service receives)

```json
{
  "sub":        "user-uuid",       // the identity
  "role":       "seller",          // customer | seller | admin | driver
  "seller_id":  "seller-uuid",     // present only for sellers (their tenant)
  "scopes":     ["catalog:write"], // optional fine scopes (Phase 2)
  "exp":        1733155800,
  "iss":        "bazaar-auth"
}
```

The gateway injects verified claims into a request context object that downstream gRPC calls
propagate (as gRPC metadata). **Services trust these claims because the gateway already verified the
token** — internal traffic is on a trusted network (see §7).

---

## 3. Roles

| Role | Who | Scope |
|---|---|---|
| `customer` | a buyer | their own orders, cart, addresses, reviews, chats |
| `seller` | a vendor (tenant) | their own store, products, inventory, sub-orders, payouts |
| `admin` | platform staff | cross-tenant moderation, approvals, fee rules, refunds |
| `driver` | last-mile courier | only shipments assigned to them |

A single human can hold more than one role (a seller is also a customer). Roles are claims; the active
role is determined per request by the route group.

---

## 4. Permission Model (RBAC now, ABAC later)

**Launch = RBAC.** A role maps to a fixed set of permissions (`product:write`, `order:read`,
`payout:read`, …). Simple, auditable, enough for four roles.

**Phase 2 = ABAC / policy engine (OPA)** *only if* rules outgrow roles — e.g. seller sub-accounts with
granular staff permissions, region-scoped admins, or time-boxed access. We do **not** introduce a
policy engine before the rules demand it (innovation budget, `ARCHITECTURE_PRINCIPLES.md` §5).

Permission constant naming: `<context>:<resource>:<action>` → `catalog:product:write`,
`commerce:order:cancel`, `marketplace:payout:read`.

---

## 5. Resource Ownership — the part most systems get wrong

A valid `seller` token does **not** authorize editing *any* product — only the seller's own. This
check lives in the owning service, against the owning data, on **every** mutating and
resource-scoped read.

```go
// In product-service, NOT in the gateway:
func (s *Service) UpdateProduct(ctx context.Context, productID uuid.UUID, in UpdateInput) error {
    claims := authctx.From(ctx)                 // {sub, role, seller_id}
    p, err := s.repo.GetProduct(ctx, productID)
    if err != nil { return err }

    // ownership gate
    if claims.Role != "admin" && p.SellerID != claims.SellerID {
        return ErrForbidden                     // 403 — valid token, wrong owner
    }
    return s.repo.Update(ctx, productID, in)
}
```

Ownership rules per resource:

| Resource | Owner check |
|---|---|
| product / inventory | `resource.seller_id == claims.seller_id` (or admin) |
| order / cart / address | `resource.user_id == claims.sub` (or admin) |
| sub-order (seller view) | `suborder.seller_id == claims.seller_id` |
| shipment (driver) | `shipment.driver_id == claims.sub` |
| review | author == `claims.sub` for edit; seller of target for reply |
| chat room | `claims.sub ∈ {room.customer_id, room.seller_id}` |
| payout / earnings | `payout.seller_id == claims.seller_id` |

> **Rule:** ownership is enforced by the service that owns the row. The gateway never attempts it
> (it can't — it doesn't have the data). This is also why we never share databases.

---

## 6. Permission Matrix

`R` = read own, `W` = write own, `—` = denied, `A` = all (cross-tenant).

| Capability | customer | seller | driver | admin |
|---|---|---|---|---|
| Browse catalog / search | R | R | — | A |
| Manage own products/inventory | — | W | — | A |
| Approve/reject products | — | — | — | A |
| Cart & checkout | W | — | — | — |
| View own orders | R | — | — | A |
| View seller sub-orders | — | R | — | A |
| Cancel order | W (pre-ship) | — | — | A |
| Refund | — | — | — | A |
| Update shipment checkpoints/GPS | — | — | W (assigned) | A |
| Track a parcel | R (own order) | R (own suborder) | R (assigned) | A |
| Write review | W (purchased) | — | — | A (hide) |
| Reply to review | — | W (own store) | — | A |
| Chat | W (own rooms) | W (own rooms) | — | A (support) |
| Seller onboarding / KYC | — | W (own) | — | A (approve) |
| View earnings / payouts | — | R (own) | — | A |
| Set fee rules | — | — | — | A |

---

## 7. Service-to-Service Auth

Internal gRPC calls are trusted but not unguarded:

- **Network trust boundary:** internal traffic runs on a private network / service mesh. In Phase 2,
  **mTLS** between services (mesh-issued certs) authenticates the *caller service*.
- **Propagated user context:** the originating user's claims travel as gRPC metadata so the owning
  service can still do ownership checks (a gateway-authenticated `customer` calling
  order→delivery still carries `sub`).
- **No service may forge a role it wasn't given.** Services validate that propagated claims came
  through the gateway (signed context / mesh identity), not from an arbitrary caller.
- **Kafka events carry actor info** in the envelope (`data.actor_id`, `data.actor_role`) for audit,
  but consumers act on the event, not on the actor's live permissions (the event already happened).

---

## 8. Special Cases: Driver, Admin, Impersonation, Seller Staff

- **Driver:** scoped to assigned shipments only. The driver app's checkpoint/GPS endpoints validate
  `shipment.driver_id == claims.sub` in delivery-service before accepting a scan.
- **Admin:** cross-tenant. Every admin action is written to an **append-only audit log** (reuse the
  CashPilotX `operator_audit_log` pattern) — who, what, when, target. Non-negotiable for refunds,
  suspensions, fee changes, review moderation.
- **Impersonation (support):** an admin may act *as* a user for support. This issues a short,
  clearly-flagged token (`act_as` claim) and is **always** audited as "admin X acting as user Y."
- **Seller staff (Phase 2):** a seller may invite staff with sub-permissions (`product:write` but not
  `payout:read`). This is the trigger to move from RBAC → ABAC (§4).

---

## 9. Multi-Tenant Isolation (sellers)

Sellers are tenants; their data must never leak across the boundary.

- **Every seller-scoped query filters by `seller_id`** — enforced in the repository layer, not left to
  callers. A missing tenant filter is a security bug, gated in review.
- **Kafka keys by `seller_id`/`sub_order_id`** keep one tenant's events ordered and isolatable.
- **Noisy-neighbor** (one huge seller): plan partition/index routing so a megastore can't starve
  others (`ARCHITECTURE_PRINCIPLES.md` §7.3).
- **Cross-tenant reads are admin-only** and audited.

---

## 10. Rules & Anti-Patterns

**Rules**
1. Coarse authz at the gateway (role + valid token); fine-grained ownership in the owning service.
2. Ownership is checked against owned data, on every mutating/resource-scoped request.
3. Repository layer enforces tenant (`seller_id`) / owner (`user_id`) filters — never the caller.
4. All admin and impersonation actions are written to the append-only audit log.
5. No card PAN, ever (see `ARCHITECTURE_PRINCIPLES.md` §7.2) — payment-service handles provider tokens.
6. Permissions named `<context>:<resource>:<action>`; RBAC at launch, ABAC only when rules demand it.

**Anti-patterns (rejected in review)**
- ❌ Trusting `seller_id`/`user_id` from the **request body or query** instead of the verified token.
- ❌ Doing ownership checks at the gateway (it lacks the data; it will be wrong).
- ❌ A service accepting a role claim from an arbitrary caller without mesh/gateway provenance.
- ❌ Cross-tenant queries without an admin gate + audit entry.
- ❌ Long-lived access tokens "to avoid refresh complexity."

---

*Cross-cutting · implements `ARCHITECTURE_PRINCIPLES.md` §7.1 · see also `02_auth_service.md`, `01_api_gateway.md`*
