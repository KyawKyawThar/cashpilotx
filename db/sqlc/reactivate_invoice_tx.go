package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrInvoiceNotCanceled = errors.New("invoice is not canceled")

type ReactivateInvoiceTxParams struct {
	InvoiceID  uuid.UUID
	UserID     uuid.UUID
	CustomerID uuid.UUID
}

type ReactivateInvoiceTxResult struct {
	Invoice  Invoice
	Customer Customer
}

// reactivateInvoice sets status back to unpaid/partial/paid based on paid_amount
// and resets collection_state to 'pending'. The WHERE status='cancelled' acts as
// an idempotency guard — ErrNoRows → ErrInvoiceNotCanceled.
const reactivateInvoiceSQL = `
UPDATE invoices
SET
    status = CASE
        WHEN paid_amount <= 0       THEN 'unpaid'
        WHEN paid_amount < amount   THEN 'partial'
        ELSE                             'paid'
    END,
    collection_state = 'pending',
    updated_at       = now()
WHERE id      = $1
  AND user_id = $2
  AND status  = 'cancelled'
RETURNING id, user_id, customer_id, amount, currency, paid_amount, paid_at,
          description, due_date, status, source, photo_url, external_id, issued_at,
          items, collection_state, last_contacted_at, last_response_at,
          follow_up_count, confidence_score, confidence_factors,
          outcome_text, outcome_timeframe, confidence_computed_at,
          created_at, updated_at, promised_at
`

// ReactivateInvoiceTx is the mirror of CancelInvoiceTx:
// 1. Restores invoice status (unpaid/partial/paid) and resets collection_state to 'pending'.
// 2. Adds the unpaid balance back to customer.total_owed.
// 3. Emits invoice_reactivated for the History timeline.
func (s *SQLStore) ReactivateInvoiceTx(ctx context.Context, arg ReactivateInvoiceTxParams) (ReactivateInvoiceTxResult, error) {
	var result ReactivateInvoiceTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		row := q.db.QueryRow(ctx, reactivateInvoiceSQL, arg.InvoiceID, arg.UserID)
		if err := row.Scan(
			&result.Invoice.ID,
			&result.Invoice.UserID,
			&result.Invoice.CustomerID,
			&result.Invoice.Amount,
			&result.Invoice.Currency,
			&result.Invoice.PaidAmount,
			&result.Invoice.PaidAt,
			&result.Invoice.Description,
			&result.Invoice.DueDate,
			&result.Invoice.Status,
			&result.Invoice.Source,
			&result.Invoice.PhotoUrl,
			&result.Invoice.ExternalID,
			&result.Invoice.IssuedAt,
			&result.Invoice.Items,
			&result.Invoice.CollectionState,
			&result.Invoice.LastContactedAt,
			&result.Invoice.LastResponseAt,
			&result.Invoice.FollowUpCount,
			&result.Invoice.ConfidenceScore,
			&result.Invoice.ConfidenceFactors,
			&result.Invoice.OutcomeText,
			&result.Invoice.OutcomeTimeframe,
			&result.Invoice.ConfidenceComputedAt,
			&result.Invoice.CreatedAt,
			&result.Invoice.UpdatedAt,
			&result.Invoice.PromisedAt,
		); errors.Is(err, pgx.ErrNoRows) {
			return ErrInvoiceNotCanceled
		} else if err != nil {
			return err
		}

		// Add the unpaid balance back to customer.total_owed (mirror of cancel's negative delta).
		unpaid := result.Invoice.Amount - result.Invoice.PaidAmount
		if unpaid > 0 {
			var err error
			result.Customer, err = q.AddCustomerDebt(ctx, AddCustomerDebtParams{
				ID:        arg.CustomerID,
				UserID:    arg.UserID,
				TotalOwed: unpaid,
			})
			if err != nil {
				return err
			}
		}

		// Emit invoice_reactivated for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"amount":%d,"currency":%q}`,
			result.Invoice.Amount, result.Invoice.Currency,
		))
		invID := result.Invoice.ID
		_, err := q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     arg.UserID,
			CustomerID: arg.CustomerID,
			InvoiceID:  &invID,
			EventType:  CustomerEventType("invoice_reactivated"),
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
