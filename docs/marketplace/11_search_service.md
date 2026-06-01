# 11 · Search Service

> Full-text + faceted product search across all sellers. Read-optimized projection of the catalog,
> kept in sync by consuming `product.updated` from Kafka.
> Stack: Go · Chi · Elasticsearch (or OpenSearch) · Kafka (consumer)

---

## 1. Responsibility

- Index every approved product for fast full-text + filtered search.
- Faceted filters: category, price range, brand, rating, attributes, seller.
- Autocomplete / suggestions.
- Stay eventually-consistent with product-service via Kafka — **never** queries product-service's DB.

---

## 2. Why a separate service

Product-service's Postgres is great for transactional writes, bad for fuzzy "blue cotton shirt under
500 THB rated 4★+". Search-service owns an Elasticsearch index that is a **read model** rebuilt from
the `product.updated` compacted topic — disposable and replayable.

---

## 3. Index document shape

```json
{
  "product_id": "uuid",
  "seller_id":  "uuid",
  "store_name": "Acme",
  "title":      "Cotton T-Shirt",
  "description":"...",
  "brand":      "Acme",
  "category_path": "fashion/men/tops",
  "attributes": { "material": "cotton", "color": ["red","blue"] },
  "price_minor": 25000,
  "currency":   "THB",
  "rating_avg": 4.6,
  "rating_count": 230,
  "in_stock":   true,
  "status":     "approved",
  "updated_at": "2026-06-02T..."
}
```

`in_stock` is updated from `inventory.snapshot`; `rating_avg` from `review.created` (denormalized for ranking).

---

## 4. Kafka

| Direction | Topic | Action |
|---|---|---|
| Consume | `product.updated` | Upsert/delete doc (status `approved`→index, else remove) |
| Consume | `inventory.snapshot` | Update `in_stock` flag |
| Consume | `review.created` | Update `rating_avg`, `rating_count` |
| Consume | `seller.suspended` | Remove that seller's docs |

All consumers idempotent on `event_id`. Because `product.updated` is **compacted**, a brand-new
search-service can replay the whole topic from the start and rebuild the entire index.

---

## 5. API routes

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/search` | any | `?q=&category=&min=&max=&brand=&rating=&sort=&page=` |
| GET | `/api/v1/search/suggest` | any | Autocomplete `?q=` |
| GET | `/api/v1/search/facets` | any | Available filters for a query |

**Sort options:** relevance, price asc/desc, rating, newest, best-selling (needs order data — see open items).

---

## 6. Open items

- ⚠ "Best-selling" sort needs sales counts — either consume `order.paid` to maintain a sales counter, or defer this sort.
- ⚠ Elasticsearch vs OpenSearch (license) and managed vs self-hosted — decide in Sprint 1.
- ⚠ Multi-language analyzer (Thai/Burmese tokenization) for proper full-text in local scripts.
