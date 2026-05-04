-- name: CreateUser :one
INSERT INTO users (
    phone,
    pin_hash,
    name,
    business_name,
    locale,
    plan,
    home_currency
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1
  AND is_active = true
LIMIT 1;

-- name: GetUserByPhone :one
SELECT *
FROM users
WHERE phone = $1
  AND is_active = true
LIMIT 1;



-- name: UpdateUserProfile :one
UPDATE users
SET
    name = COALESCE(sqlc.narg(name), name),
    business_name = COALESCE(sqlc.narg(business_name), business_name),
    locale = COALESCE(sqlc.narg(locale), locale),
    updated_at = now()
WHERE id = $1
  AND is_active = true
RETURNING *;

-- name: UpdateUserPlan :one
UPDATE users
SET
    plan = $2,
    updated_at = now()
WHERE id = $1
  AND is_active = true
RETURNING *;

-- name: UpdateUserStreak :one
UPDATE users
SET
    streak_days = $2,
    streak_best = GREATEST(streak_best, $2),
    updated_at = now()
WHERE id = $1
  AND is_active = true
RETURNING *;



-- name: UpdateOnboardingStep :one
-- Phase 1 flow: 0=new  1=currency set  2=channel set  3=complete
-- (step 2 was "ai_key set" in arch doc — deferred to Phase 2 / BYOK)
UPDATE users
SET
    onboarding_step    = $2,
    onboarding_done_at = CASE WHEN $2 = 3::smallint THEN now() ELSE onboarding_done_at END,
    updated_at         = now()
WHERE id = $1
  AND is_active = true
RETURNING *;


-- name: NextInvoiceSequence :one
-- Atomically increments invoice_sequence and returns the new value.
-- Used by CreateInvoiceTx to generate "INV-{year}-{seq:03d}" when the
-- caller does not supply an external_id.
UPDATE users
SET
    invoice_sequence = invoice_sequence + 1,
    updated_at       = now()
WHERE id = $1
  AND is_active = true
RETURNING invoice_sequence;

-- name: DeactivateUser :exec
UPDATE users
SET
    is_active = false,
    updated_at = now()
WHERE id = $1;