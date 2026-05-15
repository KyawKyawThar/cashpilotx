package db

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SyncInvoicesParams is the parameter set for SyncInvoices.
type SyncInvoicesParams struct {
	UserID       uuid.UUID
	UpdatedAfter time.Time
	Limit        int32
}

// SyncCustomersParams is the parameter set for SyncCustomers.
type SyncCustomersParams struct {
	UserID       uuid.UUID
	UpdatedAfter time.Time
	Limit        int32
}

const syncInvoices = `
SELECT id, user_id, customer_id, amount, currency, paid_amount, paid_at,
       description, due_date, status, source, photo_url, external_id, issued_at,
       items, collection_state, last_contacted_at, last_response_at,
       follow_up_count, confidence_score, confidence_factors,
       outcome_text, outcome_timeframe, confidence_computed_at,
       created_at, updated_at, promised_at
FROM invoices
WHERE user_id = $1
  AND updated_at > $2
ORDER BY updated_at ASC
LIMIT $3
`

const syncCustomers = `
SELECT id, user_id, name, normalized_name, phone, normalized_phone, country,
       type, status, total_owed, total_paid, credit_limit, last_activity,
       notes, tags, created_at, updated_at, customer_number
FROM customers
WHERE user_id = $1
  AND updated_at > $2
ORDER BY updated_at ASC
LIMIT $3
`

// SyncInvoices returns invoices for the given user updated after the cursor.
func (q *Queries) SyncInvoices(ctx context.Context, arg SyncInvoicesParams) ([]Invoice, error) {
	rows, err := q.db.Query(ctx, syncInvoices, arg.UserID, arg.UpdatedAfter, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Invoice{}
	for rows.Next() {
		var i Invoice
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.CustomerID,
			&i.Amount,
			&i.Currency,
			&i.PaidAmount,
			&i.PaidAt,
			&i.Description,
			&i.DueDate,
			&i.Status,
			&i.Source,
			&i.PhotoUrl,
			&i.ExternalID,
			&i.IssuedAt,
			&i.Items,
			&i.CollectionState,
			&i.LastContactedAt,
			&i.LastResponseAt,
			&i.FollowUpCount,
			&i.ConfidenceScore,
			&i.ConfidenceFactors,
			&i.OutcomeText,
			&i.OutcomeTimeframe,
			&i.ConfidenceComputedAt,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.PromisedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// SyncCustomers returns customers for the given user updated after the cursor.
func (q *Queries) SyncCustomers(ctx context.Context, arg SyncCustomersParams) ([]Customer, error) {
	rows, err := q.db.Query(ctx, syncCustomers, arg.UserID, arg.UpdatedAfter, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Customer{}
	for rows.Next() {
		var i Customer
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.Name,
			&i.NormalizedName,
			&i.Phone,
			&i.NormalizedPhone,
			&i.Country,
			&i.Type,
			&i.Status,
			&i.TotalOwed,
			&i.TotalPaid,
			&i.CreditLimit,
			&i.LastActivity,
			&i.Notes,
			&i.Tags,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.CustomerNumber,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
