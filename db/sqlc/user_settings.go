package db

import (
	"context"

	"github.com/google/uuid"
)

const updateHomeCurrency = `
UPDATE users
SET    home_currency = $2,
       locale        = CASE $2
                         WHEN 'MMK' THEN 'my'
                         WHEN 'CNY' THEN 'zh'
                         ELSE 'th'
                       END,
       updated_at    = now()
WHERE  id = $1
RETURNING id, phone, pin_hash, name, business_name, locale, plan, home_currency,
          streak_days, streak_best, invoice_sequence, onboarding_step, onboarding_done_at,
          is_active, created_at, updated_at, email, customer_sequence
`

type UpdateHomeCurrencyParams struct {
	ID           uuid.UUID `db:"id"            json:"id"`
	HomeCurrency string    `db:"home_currency"  json:"home_currency"`
}

func (q *Queries) UpdateHomeCurrency(ctx context.Context, arg UpdateHomeCurrencyParams) (User, error) {
	row := q.db.QueryRow(ctx, updateHomeCurrency, arg.ID, arg.HomeCurrency)
	var i User
	err := row.Scan(
		&i.ID, &i.Phone, &i.PinHash, &i.Name, &i.BusinessName,
		&i.Locale, &i.Plan, &i.HomeCurrency,
		&i.StreakDays, &i.StreakBest, &i.InvoiceSequence,
		&i.OnboardingStep, &i.OnboardingDoneAt,
		&i.IsActive, &i.CreatedAt, &i.UpdatedAt, &i.Email, &i.CustomerSequence,
	)
	return i, err
}
