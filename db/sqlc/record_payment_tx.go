package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	ErrPaymentAmountInvalid  = errors.New("payment amount must be positive")
	ErrPaymentExceedsBalance = errors.New("payment amount exceeds remaining invoice balance")
)

// RecordPaymentTx atomically inserts a payment, recomputes the invoice status
// from the payments table (source of truth), updates the customer's running
// totals, flips collection_state to 'paid' when the invoice is fully settled,
// and emits a payment_received event for the History timeline.

type RecordPaymentTxParams struct {
	Payment    CreatePaymentParams
	CustomerID uuid.UUID
}

type RecordPaymentTxResult struct {
	Payment  Payment
	Invoice  Invoice
	Customer Customer
}

func (s *SQLStore) RecordPaymentTx(ctx context.Context, arg RecordPaymentTxParams) (RecordPaymentTxResult, error) {
	var result RecordPaymentTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error

		// 1. Load the invoice to validate the payment amount.
		invoice, err := q.GetInvoiceByID(ctx, GetInvoiceByIDParams{
			ID:     arg.Payment.InvoiceID,
			UserID: arg.Payment.UserID,
		})
		if err != nil {
			return err
		}

		// Inherit currency from the invoice — payments always settle in the invoice currency.
		arg.Payment.Currency = invoice.Currency

		remaining := invoice.Amount - invoice.PaidAmount
		if arg.Payment.Amount <= 0 {
			return ErrPaymentAmountInvalid
		}
		if arg.Payment.Amount > remaining {
			return fmt.Errorf("%w: payment %d, remaining %d",
				ErrPaymentExceedsBalance, arg.Payment.Amount, remaining)
		}

		// 2. Insert the payment row.
		result.Payment, err = q.CreatePayment(ctx, arg.Payment)
		if err != nil {
			return err
		}

		// 3. Recompute paid_amount and status from the payments table.
		//    RecomputeInvoiceStatus: paid_amount = SUM(payments), then
		//    derives status = unpaid | partial | paid.
		result.Invoice, err = q.RecomputeInvoiceStatus(ctx, RecomputeInvoiceStatusParams{
			ID:     arg.Payment.InvoiceID,
			UserID: arg.Payment.UserID,
		})
		if err != nil {
			return err
		}

		// 4. Flip collection_state to 'paid' when the invoice is fully settled.
		if result.Invoice.Status == "paid" {
			result.Invoice, err = q.UpdateInvoiceCollectionState(ctx, UpdateInvoiceCollectionStateParams{
				ID:              result.Invoice.ID,
				UserID:          result.Invoice.UserID,
				CollectionState: "paid",
			})
			if err != nil {
				return err
			}
		}

		// 5. Update customer running totals.
		result.Customer, err = q.AddCustomerPayment(ctx, AddCustomerPaymentParams{
			ID:        arg.CustomerID,
			UserID:    arg.Payment.UserID,
			TotalPaid: result.Payment.Amount,
		})
		if err != nil {
			return err
		}

		// 6. Emit payment_received for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"amount":%d,"invoice_id":%q,"currency":%q}`,
			result.Payment.Amount, result.Invoice.ID.String(), result.Payment.Currency,
		))
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     result.Invoice.UserID,
			CustomerID: result.Invoice.CustomerID,
			InvoiceID:  &result.Invoice.ID,
			EventType:  CustomerEventTypePaymentReceived,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
