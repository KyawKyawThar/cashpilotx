package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrPaymentAlreadyVoided = errors.New("payment is already voided")

type VoidPaymentTxParams struct {
	PaymentID  uuid.UUID
	UserID     uuid.UUID
	CustomerID uuid.UUID
}

type VoidPaymentTxResult struct {
	Payment  Payment
	Invoice  Invoice
	Customer Customer
}

// VoidPaymentTx atomically voids a payment, recomputes the invoice status from
// the remaining non-voided payments, adjusts customer running totals, and emits
// a payment_voided event on the customer timeline.
func (s *SQLStore) VoidPaymentTx(ctx context.Context, arg VoidPaymentTxParams) (VoidPaymentTxResult, error) {
	var result VoidPaymentTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error

		// 1. Fetch the payment (also confirms ownership via user_id).
		payment, err := q.GetPaymentByID(ctx, GetPaymentByIDParams{
			ID:     arg.PaymentID,
			UserID: arg.UserID,
		})
		if err != nil {
			return err
		}
		if payment.VoidedAt != nil {
			return ErrPaymentAlreadyVoided
		}

		// 2. Mark as voided.
		result.Payment, err = q.SetPaymentVoided(ctx, SetPaymentVoidedParams{
			ID:     arg.PaymentID,
			UserID: arg.UserID,
		})
		if err != nil {
			return err
		}

		// 3. Recompute invoice paid_amount / status from remaining non-voided payments.
		result.Invoice, err = q.RecomputeInvoiceStatus(ctx, RecomputeInvoiceStatusParams{
			ID:     payment.InvoiceID,
			UserID: arg.UserID,
		})
		if err != nil {
			return err
		}

		// 4. Reverse the customer running totals.
		result.Customer, err = q.SubtractCustomerPayment(ctx, SubtractCustomerPaymentParams{
			ID:        arg.CustomerID,
			UserID:    arg.UserID,
			TotalPaid: payment.Amount,
		})
		if err != nil {
			return err
		}

		// 5. Emit payment_voided event for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"amount":%d,"invoice_id":%q,"currency":%q}`,
			payment.Amount, result.Invoice.ID.String(), payment.Currency,
		))
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     arg.UserID,
			CustomerID: arg.CustomerID,
			InvoiceID:  &result.Invoice.ID,
			EventType:  CustomerEventTypePaymentVoided,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
