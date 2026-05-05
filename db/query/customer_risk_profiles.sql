-- =========================================
-- UPSERT (core write path - recompute model)
-- =========================================
-- name: UpsertCustomerRiskProfile :one
INSERT INTO customer_risk_profiles (
    user_id,
    customer_id,
    total_invoices,
    profile_confidence,
    risk_score,
    risk_level,
    avg_days_to_pay,
    invoices_on_time,
    invoices_late,
    invoices_unpaid,
    amount_on_time,
    amount_late,
    amount_unpaid,
    longest_overdue,
    late_fee_enabled,
    late_fee_pct,
    late_fee_type,
    total_late_fees,
    best_contact_day,
    best_contact_time,
    best_contact_window,
    last_calculated_at
) VALUES (
    $1,
    $2,
    $3,
    CASE
        WHEN $3 < 4 THEN 'new'
        WHEN $3 < 8 THEN 'learning'
        ELSE 'confident'
    END,
    $4,
    CASE
        WHEN $4 >= 80 THEN 'excellent'
        WHEN $4 >= 60 THEN 'good'
        WHEN $4 >= 40 THEN 'moderate'
        WHEN $4 >= 20 THEN 'risky'
        ELSE 'bad'
    END,
    $5,
    $6,   -- invoices_on_time  (count)
    $7,   -- invoices_late     (count)
    $8,   -- invoices_unpaid   (count)
    $9,   -- amount_on_time    (minor units)
    $10,  -- amount_late       (minor units)
    $11,  -- amount_unpaid     (minor units)
    $12,  -- longest_overdue   (days)
    $13,  -- late_fee_enabled
    $14,  -- late_fee_pct
    $15,  -- late_fee_type
    $16,  -- total_late_fees
    $17,  -- best_contact_day   e.g. 'Friday'
    $18,  -- best_contact_time  e.g. '16:00-18:00'
    $19,  -- best_contact_window e.g. 'Friday 4–6 pm'
    now()
)
ON CONFLICT (user_id, customer_id)
DO UPDATE SET
    total_invoices      = EXCLUDED.total_invoices,
    profile_confidence  = EXCLUDED.profile_confidence,
    risk_score          = EXCLUDED.risk_score,
    risk_level          = EXCLUDED.risk_level,
    avg_days_to_pay     = EXCLUDED.avg_days_to_pay,
    invoices_on_time    = EXCLUDED.invoices_on_time,
    invoices_late       = EXCLUDED.invoices_late,
    invoices_unpaid     = EXCLUDED.invoices_unpaid,
    amount_on_time      = EXCLUDED.amount_on_time,
    amount_late         = EXCLUDED.amount_late,
    amount_unpaid       = EXCLUDED.amount_unpaid,
    longest_overdue     = EXCLUDED.longest_overdue,
    late_fee_enabled    = EXCLUDED.late_fee_enabled,
    late_fee_pct        = EXCLUDED.late_fee_pct,
    late_fee_type       = EXCLUDED.late_fee_type,
    total_late_fees     = EXCLUDED.total_late_fees,
    best_contact_day    = EXCLUDED.best_contact_day,
    best_contact_time   = EXCLUDED.best_contact_time,
    best_contact_window = EXCLUDED.best_contact_window,
    last_calculated_at  = now(),
    updated_at          = now()
RETURNING *;

-- name: UpdateContactWindow :exec
-- Lightweight write: called after RefreshRiskProfileTx detects a new payment-day
-- pattern without recomputing the full risk score.
UPDATE customer_risk_profiles
SET
    best_contact_day    = $3,
    best_contact_time   = $4,
    best_contact_window = $5,
    updated_at          = now()
WHERE user_id     = $1
  AND customer_id = $2;


-- =========================================
-- READ: single profile
-- =========================================
-- name: GetCustomerRiskProfile :one
SELECT *
FROM customer_risk_profiles
WHERE user_id = $1
  AND customer_id = $2;


-- =========================================
-- READ: list all (paginated)
-- =========================================
-- name: ListCustomerRiskProfiles :many
SELECT *
FROM customer_risk_profiles
WHERE user_id = $1
ORDER BY risk_score DESC
LIMIT $2 OFFSET $3;


-- =========================================
-- READ: high risk customers
-- =========================================
-- name: ListHighRiskCustomers :many
SELECT *
FROM customer_risk_profiles
WHERE user_id = $1
  AND risk_level IN ('risky','bad')
ORDER BY risk_score ASC
LIMIT $2 OFFSET $3;


-- =========================================
-- READ: best customers
-- =========================================
-- name: ListBestCustomers :many
SELECT *
FROM customer_risk_profiles
WHERE user_id = $1
  AND risk_level = 'excellent'
ORDER BY risk_score DESC
LIMIT $2 OFFSET $3;


-- =========================================
-- UPDATE: late fee settings (validated)
-- =========================================
-- name: UpdateLateFeeSettings :exec
UPDATE customer_risk_profiles
SET
    late_fee_enabled = $3,
    late_fee_pct     = $4,
    late_fee_type    = $5,
    updated_at       = now()
WHERE user_id = $1
  AND customer_id = $2
  AND (
        $3 = FALSE OR
        ($3 = TRUE AND $4 > 0)
      );


-- =========================================
-- UPDATE: touch recalculation timestamp
-- =========================================
-- name: TouchRiskProfileCalculated :exec
UPDATE customer_risk_profiles
SET
    last_calculated_at = now(),
    updated_at         = now()
WHERE user_id = $1
  AND customer_id = $2;


-- =========================================
-- LOCK: for transactional recalculation
-- =========================================
-- name: GetCustomerRiskProfileForUpdate :one
SELECT *
FROM customer_risk_profiles
WHERE user_id = $1
  AND customer_id = $2
FOR UPDATE;


-- =========================================
-- OPTIONAL: hard delete (use carefully)
-- =========================================
-- name: DeleteCustomerRiskProfile :exec
DELETE FROM customer_risk_profiles
WHERE user_id = $1
  AND customer_id = $2;
