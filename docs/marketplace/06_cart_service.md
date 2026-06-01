# 06 · Cart Service

> The shopping cart. Multi-seller (items grouped by seller), Redis-backed, fast and ephemeral.
> Stack: Go · Chi · Redis (primary store) · gRPC clients (product, inventory)

---

## 1. Responsibility

- Hold a customer's cart: line items grouped by `seller_id` (becomes sub-orders at checkout).
- Validate price + availability at add-time and again at checkout (prices/stock drift).
- Live in **Redis** (not Postgres) — carts are high-churn, short-lived, tolerate loss.
- Hand a validated cart snapshot to order-service at checkout.

---

## 2. Storage model (Redis)

```
Key:   cart:{user_id}                      (Hash, TTL 30 days, refreshed on touch)
Value: per-line entries
  line:{sku} → { sku, product_id, seller_id, qty, unit_price_minor, title, photo_url }

Guest carts:  cart:guest:{cart_token}  → merged into cart:{user_id} on login.
```

No SQL schema — Redis only. A cart is rebuildable from product-service if lost.

---

## 3. Cart shape returned to client

```json
{
  "cart_id": "user-uuid",
  "groups": [
    {
      "seller_id": "uuid", "store_name": "Acme",
      "lines": [
        {"sku": "TSHIRT-RED-M", "title": "Tee", "qty": 2,
         "unit_price_minor": 25000, "line_total_minor": 50000, "in_stock": true}
      ],
      "group_subtotal_minor": 50000
    }
  ],
  "subtotal_minor": 50000,
  "currency": "THB"
}
```

The `groups` array maps 1:1 to the sub-orders order-service will create.

---

## 4. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/cart` | customer | Current cart (grouped by seller) |
| POST | `/api/v1/cart/items` | customer | Add/increment a SKU (validates price + stock via gRPC) |
| PATCH | `/api/v1/cart/items/{sku}` | customer | Change qty |
| DELETE | `/api/v1/cart/items/{sku}` | customer | Remove line |
| DELETE | `/api/v1/cart` | customer | Clear |
| POST | `/api/v1/cart/checkout` | customer | Re-validate all lines → return checkout snapshot for order-service |

---

## 5. gRPC usage (outbound)

```
product.GetVariant(sku)     → current price, seller_id, title  (on add + on checkout)
inventory.BatchStock([]sku) → availability flags                (on get + on checkout)
```

No Kafka — cart is pre-order; nothing has "happened" yet to broadcast.

---

## 6. Checkout handoff

```
POST /cart/checkout
  1. Re-fetch live prices (product) + stock (inventory) for every line
  2. If price changed → return 409 with new prices (client confirms)
  3. If out of stock  → return 409 listing unavailable SKUs
  4. Else → return immutable checkout snapshot {groups, totals, snapshot_id}
  5. Client POSTs snapshot_id to order-service → order created from frozen snapshot
```

Freezing the snapshot prevents price/stock changing between "Review order" and "Pay".

---

## 7. Open items

- ⚠ Guest cart merge strategy on login (union vs replace) — default: union, sum quantities.
- ⚠ Cart TTL vs reservation: cart add does NOT reserve stock; reservation happens only at order-service. Confirm this is acceptable (last-unit race resolved at order time).
