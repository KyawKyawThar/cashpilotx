-- ─── Daily Report Queries ────────────────────────────────────────────────────
-- All queries scope to a single org via org_owner_id ($1).
-- Date range: $2 = from_date (DATE), $3 = to_date (DATE).
-- Optional text filters use '' to mean "no filter".

-- name: GetProductListReport :many
-- Shows all active products for the org with aggregated sales in the date range.
-- Products with no sales in the period appear with zero values.
-- revenue    = qty * unit_price - discount  (actual income after discount)
-- total_value = qty * unit_price            (income before discount)
WITH item_agg AS (
  SELECT
    item->>'name'                                              AS product_name,
    COUNT(DISTINCT i.id)                                       AS total_order,
    COALESCE(SUM((item->>'qty')::NUMERIC), 0)                  AS total_qty,
    COALESCE(SUM(
      GREATEST((item->>'qty')::NUMERIC,        0) *
      GREATEST((item->>'unit_price')::NUMERIC, 0) -
      COALESCE(GREATEST((item->>'discount')::NUMERIC, 0), 0)
    ), 0)                                                      AS revenue,
    COALESCE(SUM(
      GREATEST((item->>'qty')::NUMERIC,        0) *
      GREATEST((item->>'unit_price')::NUMERIC, 0)
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
  COALESCE(a.revenue,     0)::BIGINT      AS revenue,
  COALESCE(a.total_value, 0)::BIGINT      AS total_value
FROM products p
LEFT JOIN item_agg a ON a.product_name = p.name
WHERE p.org_owner_id = $1
  AND p.is_active    = true
  AND ($5::text = '' OR p.name ILIKE '%' || $5 || '%')
ORDER BY COALESCE(a.total_value, 0) DESC, p.name ASC;

-- name: GetSaleReport :many
-- Shows all active team members with new-customer count, cash collected, and invoice totals.
-- Members with no activity in the period appear with zero values.
WITH
cust_agg AS (
  SELECT
    c.assigned_to                   AS user_id,
    COUNT(c.id)::BIGINT             AS total_customer
  FROM customers c
  WHERE c.user_id        = $1
    AND c.assigned_to    IS NOT NULL
    AND c.created_at::date >= $2 AND c.created_at::date < $3
  GROUP BY c.assigned_to
),
pay_agg AS (
  SELECT
    i.assigned_to                       AS user_id,
    COALESCE(SUM(p.amount), 0)::BIGINT  AS cash_revenue
  FROM payments p
  JOIN invoices i ON i.id = p.invoice_id AND i.user_id = $1
  WHERE p.paid_on   >= $2 AND p.paid_on  < $3
    AND p.voided_at IS NULL
    AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
  GROUP BY i.assigned_to
),
inv_agg AS (
  SELECT
    i.assigned_to                                  AS user_id,
    COUNT(DISTINCT i.id)::BIGINT                   AS invoice_count,
    COALESCE(SUM(i.amount), 0)::BIGINT             AS invoice_revenue,
    COUNT(DISTINCT i.id)::BIGINT                   AS total_order,
    COALESCE(SUM(i.amount), 0)::BIGINT             AS total_value
  FROM invoices i
  WHERE i.user_id        = $1
    AND i.issued_at::date >= $2 AND i.issued_at::date < $3
    AND i.status          != 'cancelled'
    AND ($4::text = '' OR i.tracking_number ILIKE '%' || $4 || '%')
  GROUP BY i.assigned_to
)
SELECT
  u.name                                              AS sale_name,
  COALESCE(ca.total_customer,  0)::BIGINT             AS total_customer,
  COALESCE(pa.cash_revenue,    0)::BIGINT             AS cash_revenue,
  COALESCE(ia.invoice_count,   0)::BIGINT             AS invoice_count,
  COALESCE(ia.invoice_revenue, 0)::BIGINT             AS invoice_revenue,
  COALESCE(ia.total_order,     0)::BIGINT             AS total_order,
  COALESCE(ia.total_value,     0)::BIGINT             AS total_value
FROM users u
LEFT JOIN cust_agg ca ON ca.user_id = u.id
LEFT JOIN pay_agg  pa ON pa.user_id = u.id
LEFT JOIN inv_agg  ia ON ia.user_id = u.id
WHERE u.org_owner_id = $1
  AND u.role         IN ('cs_team', 'sales')
  AND u.is_active    = true
  AND ($5::text = '' OR u.name ILIKE '%' || $5 || '%')
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
                    AND i.status         NOT IN ('cancelled', 'returned')
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

-- name: GetTopOverdueCustomers :many
-- Top 10 customers with the largest outstanding balance from invoices in the date range.
SELECT
  c.name                                              AS customer_name,
  COALESCE(c.phone, '')                               AS phone,
  COALESCE(u.name,  '')                               AS sale_name,
  COUNT(DISTINCT i.id)::BIGINT                        AS invoice_count,
  COALESCE(SUM(i.amount - i.paid_amount), 0)::BIGINT  AS outstanding_amount,
  MAX(CURRENT_DATE - i.issued_at::date)::BIGINT       AS max_days_overdue
FROM invoices i
JOIN customers c ON c.id = i.customer_id AND c.user_id = $1
LEFT JOIN users u ON u.id = i.assigned_to
WHERE i.user_id          = $1
  AND i.issued_at::date >= $2
  AND i.issued_at::date  < $3
  AND i.status NOT IN ('cancelled', 'returned', 'paid')
  AND i.paid_amount      < i.amount
GROUP BY c.id, c.name, c.phone, u.name
ORDER BY outstanding_amount DESC
LIMIT 10;

-- name: GetCollectionStats :one
-- Collection pipeline statistics for invoices in the date range.
SELECT
  COUNT(*) FILTER (WHERE collection_state NOT IN ('pending', 'defaulted'))::BIGINT AS total_in_collection,
  COUNT(*) FILTER (WHERE collection_state = 'paid')::BIGINT                        AS collection_paid,
  COUNT(*) FILTER (WHERE collection_state = 'promised')::BIGINT                   AS currently_promised,
  COUNT(*) FILTER (WHERE collection_state = 'defaulted')::BIGINT                  AS defaulted_count
FROM invoices
WHERE user_id          = $1
  AND issued_at::date >= $2
  AND issued_at::date  < $3
  AND status NOT IN ('cancelled', 'returned');

-- name: GetAgingReport :one
-- Outstanding amounts for invoices issued in the date range, bucketed by age as of today.
SELECT
  COALESCE(SUM(i.amount - i.paid_amount) FILTER (
    WHERE (CURRENT_DATE - i.issued_at::date) <= 30
  ), 0)::BIGINT AS days_0_30,
  COALESCE(SUM(i.amount - i.paid_amount) FILTER (
    WHERE (CURRENT_DATE - i.issued_at::date) BETWEEN 31 AND 60
  ), 0)::BIGINT AS days_31_60,
  COALESCE(SUM(i.amount - i.paid_amount) FILTER (
    WHERE (CURRENT_DATE - i.issued_at::date) BETWEEN 61 AND 90
  ), 0)::BIGINT AS days_61_90,
  COALESCE(SUM(i.amount - i.paid_amount) FILTER (
    WHERE (CURRENT_DATE - i.issued_at::date) > 90
  ), 0)::BIGINT AS days_90_plus
FROM invoices i
WHERE i.user_id          = $1
  AND i.issued_at::date >= $2
  AND i.issued_at::date  < $3
  AND i.status NOT IN ('cancelled', 'returned')
  AND i.paid_amount      < i.amount;
