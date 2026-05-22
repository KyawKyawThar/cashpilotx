package db

import (
	"context"

	"github.com/google/uuid"
)

const updateUserRole = `
UPDATE users
SET    role       = $2,
       updated_at = now()
WHERE  id = $1
  AND  is_active  = true
RETURNING id, phone, pin_hash, name, business_name, locale, plan, home_currency,
          streak_days, streak_best, invoice_sequence, onboarding_step, onboarding_done_at,
          is_active, created_at, updated_at, email, customer_sequence,
          username, force_pin_change, role, team_id, created_by, org_owner_id
`

type UpdateUserRoleParams struct {
	ID   uuid.UUID `db:"id"   json:"id"`
	Role UserRole  `db:"role" json:"role"`
}

func (q *Queries) UpdateUserRole(ctx context.Context, arg UpdateUserRoleParams) (User, error) {
	row := q.db.QueryRow(ctx, updateUserRole, arg.ID, arg.Role)
	var i User
	err := row.Scan(
		&i.ID, &i.Phone, &i.PinHash, &i.Name, &i.BusinessName,
		&i.Locale, &i.Plan, &i.HomeCurrency,
		&i.StreakDays, &i.StreakBest, &i.InvoiceSequence,
		&i.OnboardingStep, &i.OnboardingDoneAt,
		&i.IsActive, &i.CreatedAt, &i.UpdatedAt, &i.Email, &i.CustomerSequence,
		&i.Username, &i.ForcePinChange, &i.Role,
		&i.TeamID, &i.CreatedBy, &i.OrgOwnerID,
	)
	return i, err
}
