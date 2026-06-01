# 01 · API Gateway

> Edge of the platform. Stateless. Verifies tokens, rate-limits, routes, and aggregates.
> Stack: Go · Chi · Redis (rate limit) · PASETO verify · gRPC clients to internal services

---

## 1. Responsibility

- **Single entry point** for all web/mobile/seller-center clients (REST + WSS + SSE).
- **Auth gate:** verify PASETO access token locally (auth-service's public key) — no per-request call.
- **Rate limiting:** Redis sliding-window per IP + per user + per route class.
- **Routing:** map public REST paths → internal gRPC/REST services.
- **BFF aggregation:** combine calls for screens that need data from several services (e.g. product page = product + inventory + reviews + storefront).
- **Realtime proxy:** terminate client WS/SSE and proxy to chat/delivery/order.
- Owns **no business data**. Only touches Redis for rate-limit counters.

---

## 2. What it does NOT do

- No business logic, no DB of its own (beyond Redis counters).
- No token issuance (that's auth-service) — only verification.
- No Kafka producing/consuming.

---

## 3. Token verification middleware

```go
// PASETO v4 public verify — key fetched from auth-service /jwks-equivalent at boot + refresh.
func AuthMiddleware(pub ed25519.PublicKey) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tok := bearer(r)
            claims, err := paseto.VerifyV4Public(tok, pub)   // local, no network
            if err != nil { writeErr(w, 401, "invalid_token"); return }
            ctx := withClaims(r.Context(), claims)            // sub, role, seller_id?
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

Roles in claims: `customer | seller | admin | driver`. Route groups enforce role.

---

## 4. Rate limiting (Redis sliding window)

| Route class | Limit | Key |
|---|---|---|
| `POST /auth/*` | 10 / min | IP |
| `POST /orders` (checkout) | 5 / min | user_id |
| `GET /search` | 60 / min | user_id |
| `POST /driver/location` | 120 / min | driver_id |
| default | 300 / min | user_id |

Returns `429` + `Retry-After` when exceeded.

---

## 5. Public route map (selected)

| Public path | → routes to | Role |
|---|---|---|
| `POST /api/v1/auth/*` | auth-service | — |
| `GET /api/v1/products` | product-service (+ inventory, review aggregate) | any |
| `GET /api/v1/products/{id}` | BFF: product + inventory + reviews + storefront | any |
| `GET /api/v1/search` | search-service | any |
| `POST /api/v1/cart/*` | cart-service | customer |
| `POST /api/v1/orders` | order-service | customer |
| `GET /api/v1/orders/{id}/stream` | SSE → order-service | customer |
| `GET /api/v1/tracking/{trackingNo}` | delivery-service | customer |
| `WSS /api/v1/tracking/{trackingNo}/live` | WS → delivery-service | customer |
| `WSS /api/v1/chat` | WS → chat-service | customer/seller |
| `POST /api/v1/driver/*` | delivery-service | driver |
| `* /api/v1/seller/*` | seller/product/storefront | seller |
| `* /api/v1/admin/*` | various | admin |

---

## 6. BFF aggregation example — product detail page

```
GET /api/v1/products/{id}
  ├─ gRPC product.Get(id)            → name, price, variants, photos
  ├─ gRPC inventory.GetStock(id)     → in stock?
  ├─ gRPC review.Summary(productId)  → avg rating, count
  └─ gRPC storefront.Get(sellerId)   → store name, badge
  → merged JSON in one response (calls run concurrently, 200ms budget, partial on timeout)
```

---

## 7. Cross-cutting

- Injects `X-Request-Id`; starts OTel span; propagates `trace-id` into gRPC metadata.
- CORS for the PWA origin(s).
- Idempotency-Key pass-through to downstream mutating services.

---

## 8. Open items

- ⚠ Gateway-local PASETO key refresh cadence — see master ⚠ GAP for auth. Cache public key, refresh hourly.
- BFF timeout/partial-response policy per screen to be tuned in Sprint 1.
