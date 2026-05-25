-- ─── Daily Report Queries ────────────────────────────────────────────────────
-- All queries scope to a single org via org_owner_id ($1).
-- Date range: $2 = from_date (DATE), $3 = to_date (DATE).
-- Optional text filters use '' to mean "no filter".

-- name: GetProductListReport :many
-- Shows all active products for the org with aggregated sales in the date range.
-- Products with no sales in the period appear with zero values.
WITH item_agg AS (
  SELECT
    item->>'name'                                              AS product_name,
    COUNT(DISTINCT i.id)                                       AS total_order,
    COALESCE(SUM((item->>'qty')::NUMERIC), 0)                  AS total_qty,
    COALESCE(SUM(
      GREATEST((item->>'qty')::NUMERIC,        0) *
      GREATEST((item->>'unit_price')::NUMERIC, 0) -
      COALESCE(GREATEST((item->>'discount')::NUMERIC, 0), 0)
    ), 0)                                                      AS total_value
  FROM invoices i
  CROSS JOIN LATERAL jsonb_array_elements(i.items) AS item
  WHERE i.user_id      = $1
    AND i.issued_at::date >= $2 AND i.issued_at::date < $3
    AND i.status       != 'cancelled'
    AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
  GROUP BY item->>'name'
)
SELECT
  p.name                                  AS product_name,
  COALESCE(a.total_order, 0)::BIGINT      AS total_order,
  COALESCE(a.total_qty,   0)::BIGINT      AS total_qty,
  COALESCE(a.total_value, 0)::BIGINT      AS revenue,
  COALESCE(a.total_value, 0)::BIGINT      AS total_value
FROM products p
LEFT JOIN item_agg a ON a.product_name = p.name
WHERE p.org_owner_id = $1
  AND p.is_active    = true
  AND ($5::text = '' OR p.name ILIKE '%' || $5 || '%')
ORDER BY COALESCE(a.total_value, 0) DESC, p.name ASC;

-- name: GetSaleReport :many
-- Shows all active team members with their invoice and payment totals.
-- Members with no activity in the period appear with zero values.
SELECT
  u.name                                                                  AS sale_name,
  COUNT(DISTINCT i.customer_id)::BIGINT                                   AS total_customer,
  COALESCE(SUM(p.amount) FILTER (WHERE p.voided_at IS NULL), 0)::BIGINT  AS cash_revenue,
  COUNT(DISTINCT i.id)::BIGINT                                            AS invoice_count,
  COALESCE(SUM(i.amount), 0)::BIGINT                                      AS invoice_revenue,
  COUNT(DISTINCT i.id)::BIGINT                                            AS total_order,
  COALESCE(SUM(i.amount), 0)::BIGINT                                      AS total_value
FROM users u
LEFT JOIN invoices i ON i.assigned_to    = u.id
                    AND i.user_id        = u.org_owner_id
                    AND i.issued_at::date >= $2 AND i.issued_at::date < $3
                    AND i.status         != 'cancelled'
                    AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
LEFT JOIN payments p ON p.invoice_id     = i.id
                    AND p.voided_at      IS NULL
WHERE u.org_owner_id = $1
  AND u.role         IN ('cs_team', 'sales')
  AND u.is_active    = true
  AND ($5::text = '' OR u.name ILIKE '%' || $5 || '%')
GROUP BY u.id, u.name
ORDER BY u.name;

-- name: GetRecoverCashReport :many
-- Cash collected on invoices that went through the collection flow,
-- grouped by the assigned salesperson.
SELECT
  u.name                                                                  AS sale_name,
  COUNT(p.id)::BIGINT                                                     AS total_order,
  COALESCE(SUM(p.amount) FILTER (WHERE p.voided_at IS NULL), 0)::BIGINT  AS amount,
  COALESCE(SUM(p.amount) FILTER (WHERE p.voided_at IS NULL), 0)::BIGINT  AS total_value
FROM users u
LEFT JOIN invoices i  ON i.assigned_to      = u.id
                     AND i.user_id          = u.org_owner_id
                     AND i.collection_state != 'pending'
                     AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
LEFT JOIN payments p  ON p.invoice_id       = i.id
                     AND p.paid_on >= $2 AND p.paid_on < $3
                     AND p.voided_at        IS NULL
WHERE u.org_owner_id = $1
  AND u.role         IN ('cs_team', 'sales')
  AND u.is_active    = true
  AND ($5::text = '' OR u.name ILIKE '%' || $5 || '%')
GROUP BY u.id, u.name
ORDER BY u.name;

-- name: GetInvoiceReport :many
-- Invoices issued in the date range, grouped by assigned salesperson.
SELECT
  u.name                                          AS sale_name,
  COUNT(DISTINCT i.id)::BIGINT                    AS total_order,
  COALESCE(SUM(i.amount), 0)::BIGINT              AS amount,
  COALESCE(SUM(i.paid_amount), 0)::BIGINT         AS paid_amount,
  COALESCE(SUM(i.amount), 0)::BIGINT              AS total_value
FROM users u
LEFT JOIN invoices i ON i.assigned_to    = u.id
                    AND i.user_id        = u.org_owner_id
                    AND i.issued_at::date >= $2 AND i.issued_at::date < $3
                    AND i.status         != 'cancelled'
                    AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
WHERE u.org_owner_id = $1
  AND u.role         IN ('cs_team', 'sales')
  AND u.is_active    = true
  AND ($5::text = '' OR u.name ILIKE '%' || $5 || '%')
GROUP BY u.id, u.name
ORDER BY u.name;

-- name: GetScheduleReport :many
-- Reminders scheduled in the date range, grouped by the invoice's assigned salesperson.
SELECT
  u.name                       AS sale_name,
  COUNT(r.id)::BIGINT          AS total_order,
  COUNT(r.id)::BIGINT          AS count,
  0::BIGINT                    AS total_value
FROM users u
LEFT JOIN invoices  i ON i.assigned_to    = u.id
                     AND i.user_id        = u.org_owner_id
                     AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
LEFT JOIN reminders r ON r.invoice_id     = i.id
                     AND r.scheduled_at::date >= $2 AND r.scheduled_at::date < $3
                     AND r.status         != 'cancelled'
WHERE u.org_owner_id = $1
  AND u.role         IN ('cs_team', 'sales')
  AND u.is_active    = true
  AND ($5::text = '' OR u.name ILIKE '%' || $5 || '%')
GROUP BY u.id, u.name
ORDER BY u.name;

-- name: GetPaymentReport :many
-- All non-voided payments recorded in the date range, grouped by the invoice's salesperson.
SELECT
  u.name                                                                  AS sale_name,
  COUNT(p.id)::BIGINT                                                     AS total_order,
  COALESCE(SUM(p.amount) FILTER (WHERE p.voided_at IS NULL), 0)::BIGINT  AS amount,
  COALESCE(SUM(p.amount) FILTER (WHERE p.voided_at IS NULL), 0)::BIGINT  AS total_value
FROM users u
LEFT JOIN invoices i  ON i.assigned_to = u.id
                     AND i.user_id     = u.org_owner_id
                     AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
LEFT JOIN payments p  ON p.invoice_id  = i.id
                     AND p.paid_on     BETWEEN $2 AND $3
                     AND p.voided_at   IS NULL
WHERE u.org_owner_id = $1
  AND u.role         IN ('cs_team', 'sales')
  AND u.is_active    = true
  AND ($5::text = '' OR u.name ILIKE '%' || $5 || '%')
GROUP BY u.id, u.name
ORDER BY u.name;
