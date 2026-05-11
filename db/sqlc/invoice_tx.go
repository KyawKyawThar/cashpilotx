package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ── CreateInvoiceTx ───────────────────────────────────────────────────────────
// Atomically creates a customer (if new), an invoice, and the first reminder.
// Set NewCustomer to nil when the customer already exists; in that case
// Invoice.CustomerID must be pre-filled by the caller.
// collection_state defaults to 'pending' via the DB column default.
// Emits an invoice_created customer event for the History timeline.

type CreateInvoiceTxParams struct {
	NewCustomer *CreateCustomerParams // nil → customer already exists
	Invoice     CreateInvoiceParams
	Reminder    CreateReminderParams
}

type CreateInvoiceTxResult struct {
	Customer Customer
	Invoice  Invoice
	Reminder Reminder
}

func (s *SQLStore) CreateInvoiceTx(ctx context.Context, arg CreateInvoiceTxParams) (CreateInvoiceTxResult, error) {

	var result CreateInvoiceTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error
		if arg.NewCustomer != nil {
			var seq int32
			seq, err = q.NextCustomerSequence(ctx, arg.NewCustomer.UserID)
			if err != nil {
				return err
			}
			num := fmt.Sprintf("C-%04d", seq)
			arg.NewCustomer.CustomerNumber = &num

			result.Customer, err = q.CreateCustomer(ctx, *arg.NewCustomer)
			if err != nil {
				return err
			}
			// propagate the new ID so both child records point to the right customer
			arg.Invoice.CustomerID = result.Customer.ID
			arg.Reminder.CustomerID = result.Customer.ID
		}

		result.Invoice, err = q.CreateInvoice(ctx, arg.Invoice)
		if err != nil {
			return err
		}

		arg.Reminder.InvoiceID = result.Invoice.ID
		arg.Reminder.CustomerID = result.Invoice.CustomerID
		result.Reminder, err = q.CreateReminder(ctx, arg.Reminder)
		if err != nil {
			return err
		}

		// Track the invoice amount in customer total_owed so CancelInvoiceTx
		// has a symmetric delta to reverse.
		result.Customer, err = q.AddCustomerDebt(ctx, AddCustomerDebtParams{
			ID:        result.Invoice.CustomerID,
			UserID:    result.Invoice.UserID,
			TotalOwed: result.Invoice.Amount,
		})
		if err != nil {
			return err
		}

		// Emit invoice_created for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"amount":%d,"currency":%q}`,
			result.Invoice.Amount, result.Invoice.Currency,
		))
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     result.Invoice.UserID,
			CustomerID: result.Invoice.CustomerID,
			InvoiceID:  &result.Invoice.ID,
			EventType:  CustomerEventTypeInvoiceCreated,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}

// CancelInvoiceTx atomically cancels an invoice, kills its pending reminders,
// and reverses the customer's total_owed by the unpaid balance. This mirrors
// what CreateInvoiceTx added to total_owed at creation time.
//
// Returns ErrInvoiceAlreadyCanceled if the invoice was already canceled
// (idempotency guard — the SQL WHERE status != 'cancelled' drives this).

var ErrInvoiceAlreadyCanceled = errors.New("invoice is already canceled")

type CancelInvoiceTxParams struct {
	InvoiceID  uuid.UUID
	UserID     uuid.UUID
	CustomerID uuid.UUID
}

type CancelInvoiceTxResult struct {
	Invoice  Invoice
	Customer Customer
}

func (s *SQLStore) CancelInvoiceTx(ctx context.Context, arg CancelInvoiceTxParams) (CancelInvoiceTxResult, error) {
	var result CancelInvoiceTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		// 1. Stop reminders before touching the invoice
		if err := q.CancelRemindersForInvoice(ctx, arg.InvoiceID); err != nil {
			return err
		}

		// 2. Cancel the invoice; WHERE status != 'cancelled' is the idempotency guard:
		//    a second call produces ErrNoRows → ErrInvoiceAlreadyCanceled.
		var err error
		result.Invoice, err = q.CancelInvoice(ctx, CancelInvoiceParams{
			ID:     arg.InvoiceID,
			UserID: arg.UserID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvoiceAlreadyCanceled
		}
		if err != nil {
			return err
		}

		// 3. Reverse the unpaid portion from customer totals.
		//    AddCustomerDebt accepts a signed delta; negative value reduces total_owed.
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

		// 4. Emit invoice_canceled for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"amount":%d,"currency":%q}`,
			result.Invoice.Amount, result.Invoice.Currency,
		))
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     result.Invoice.UserID,
			CustomerID: result.Invoice.CustomerID,
			InvoiceID:  &result.Invoice.ID,
			EventType:  CustomerEventTypeInvoiceCanceled,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}

// WriteOffInvoiceTx atomically cancels all pending reminders, marks the
// invoice canceled (status), sets collection_state='defaulted', and emits
// a collection_state_changed event so the History timeline reflects the
// write-off decision.

type WriteOffInvoiceTxParams struct {
	InvoiceID  uuid.UUID
	UserID     uuid.UUID
	CustomerID uuid.UUID
}

type WriteOffInvoiceTxResult struct {
	Invoice Invoice
}

func (s *SQLStore) WriteOffInvoiceTx(ctx context.Context, arg WriteOffInvoiceTxParams) (WriteOffInvoiceTxResult, error) {
	var result WriteOffInvoiceTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		// 1. Kill all pending/processing reminders so cron stops chasing this debt.
		if err := q.CancelRemindersForInvoice(ctx, arg.InvoiceID); err != nil {
			return err
		}

		// 2. Mark the invoice canceled (financial status).
		var err error
		result.Invoice, err = q.CancelInvoice(ctx, CancelInvoiceParams{
			ID:     arg.InvoiceID,
			UserID: arg.UserID,
		})
		if err != nil {
			return err
		}

		// 3. Set collection_state='defaulted'.
		result.Invoice, err = q.UpdateInvoiceCollectionState(ctx, UpdateInvoiceCollectionStateParams{
			ID:              arg.InvoiceID,
			UserID:          arg.UserID,
			CollectionState: "defaulted",
		})
		if err != nil {
			return err
		}

		// 4. Emit collection_state_changed for the History timeline.
		payload := json.RawMessage(`{"to":"defaulted"}`)
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     arg.UserID,
			CustomerID: arg.CustomerID,
			InvoiceID:  &arg.InvoiceID,
			EventType:  CustomerEventTypeInvoiceWrittenOff,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
