package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrInvoiceAlreadyReturned = errors.New("invoice is already returned")
var ErrInvoiceNotReturnable = errors.New("invoice cannot be returned: already paid or cancelled")

type ReturnInvoiceTxParams struct {
	InvoiceID    uuid.UUID
	UserID       uuid.UUID
	CustomerID   uuid.UUID
	ReturnReason *string
	ReturnItems  json.RawMessage
	AssignedTo   *uuid.UUID
}

type ReturnInvoiceTxResult struct {
	Invoice  Invoice
	Customer Customer
}

// ReturnInvoiceTx atomically marks an invoice as returned, cancels pending
// reminders, reverses the unpaid balance from customer total_owed, and emits
// an invoice_returned event. Payments already recorded are kept for accounting
// — the caller should display a warning if paid_amount > 0 (refund needed).
func (s *SQLStore) ReturnInvoiceTx(ctx context.Context, arg ReturnInvoiceTxParams) (ReturnInvoiceTxResult, error) {
	var result ReturnInvoiceTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		// 1. Cancel pending reminders before touching the invoice.
		if err := q.CancelRemindersForInvoice(ctx, arg.InvoiceID); err != nil {
			return err
		}

		// 2. Mark as returned; WHERE NOT IN ('paid','cancelled','returned') is the guard.
		var err error
		var returnItems *json.RawMessage
		if len(arg.ReturnItems) > 0 {
			returnItems = &arg.ReturnItems
		}
		result.Invoice, err = q.ReturnInvoice(ctx, ReturnInvoiceParams{
			ID:           arg.InvoiceID,
			UserID:       arg.UserID,
			ReturnReason: arg.ReturnReason,
			ReturnItems:  returnItems,
			AssignedTo:   arg.AssignedTo,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvoiceAlreadyReturned
		}
		if err != nil {
			return err
		}

		// 3. Reverse only the UNPAID portion from customer total_owed.
		//    paid_amount stays as-is — the caller warns the user a refund is due.
		unpaid := result.Invoice.Amount - result.Invoice.PaidAmount
		if unpaid > 0 {
			result.Customer, err = q.AddCustomerDebt(ctx, AddCustomerDebtParams{
				ID:        arg.CustomerID,
				UserID:    arg.UserID,
				TotalOwed: -unpaid,
			})
			if err != nil {
				return err
			}
		}

		// 4. Emit invoice_returned for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"amount":%d,"currency":%q,"paid_amount":%d}`,
			result.Invoice.Amount, result.Invoice.Currency, result.Invoice.PaidAmount,
		))
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     result.Invoice.UserID,
			CustomerID: result.Invoice.CustomerID,
			InvoiceID:  &result.Invoice.ID,
			EventType:  CustomerEventTypeInvoiceReturned,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
