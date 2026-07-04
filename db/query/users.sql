-- name: CreateUser :one
INSERT INTO users (
    phone,
    pin_hash,
    name,
    business_name,
    locale,
    plan,
    home_currency,
    email
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1
  AND is_active = true
  AND email IS NOT NULL
LIMIT 1;

-- name: UpdateUserPIN :exec
UPDATE users
SET
    pin_hash   = $2,
    updated_at = now()
WHERE id = $1
  AND is_active = true;



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

-- name: NextCustomerSequence :one
-- Atomically increments customer_sequence and returns the new value.
-- Used by CreateCustomerTx to assign "C-{seq:04d}" display numbers.
UPDATE users
SET
    customer_sequence = customer_sequence + 1,
    updated_at        = now()
WHERE id = $1
  AND is_active = true
RETURNING customer_sequence;

-- name: DeactivateUser :exec
UPDATE users
SET
    is_active = false,
    updated_at = now()
WHERE id = $1;

-- name: DeleteUser :exec
-- Hard-deletes the user row. All child rows (sessions, customers, invoices, etc.)
-- cascade automatically via ON DELETE CASCADE foreign keys.
-- Callers must clear Redis keys before calling this.
DELETE FROM users WHERE id = $1;
-- name: GetUserByUsername :one
SELECT *
FROM users
WHERE username = $1
  AND is_active = true
LIMIT 1;

-- name: UpdateUserPINAndClearForce :exec
UPDATE users
SET
    pin_hash         = $2,
    force_pin_change = false,
    updated_at       = now()
WHERE id = $1
  AND is_active = true;

-- name: ListUsersForDigest :many
-- Returns all active users who have an email address, for the daily digest job.
SELECT id, name, email, home_currency
FROM users
WHERE is_active = true
  AND email IS NOT NULL;

-- name: CreateTeamUser :one
INSERT INTO users (name, username, pin_hash, home_currency, plan, role, team_id, org_owner_id, created_by, force_pin_change, business_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, $10)
RETURNING *;

-- name: ListActiveOrgUsers :many
SELECT * FROM users
WHERE org_owner_id = $1
  AND id != $1
  AND is_active = true
ORDER BY role ASC, name ASC;

-- name: ListAllOrgUsers :many
SELECT * FROM users
WHERE org_owner_id = $1
  AND id != $1
ORDER BY role ASC, name ASC;

-- name: UpdateTeamUser :one
UPDATE users
SET
    name       = COALESCE(sqlc.narg(name), name),
    username   = COALESCE(sqlc.narg(username), username),
    phone      = COALESCE(sqlc.narg(phone), phone),
    team_id    = COALESCE(sqlc.narg(team_id), team_id),
    updated_at = now()
WHERE id = $1
  AND is_active = true
RETURNING *;

-- name: SetForcePinChange :exec
UPDATE users
SET
    pin_hash         = $2,
    force_pin_change = true,
    updated_at       = now()
WHERE id = $1
  AND is_active = true;

-- name: GetUserNamesByIDs :many
-- Batch-resolves user IDs to display names, e.g. for showing who a
-- customer/invoice is assigned to without a per-row join.
SELECT id, name
FROM users
WHERE id = ANY($1::uuid[]);
