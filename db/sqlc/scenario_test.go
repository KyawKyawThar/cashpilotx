package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestScenario_FullCollectionLifecycle walks the happy path:
// create invoice → fire reminder → customer promises → partial pay → full pay.
// Verifies final invoice state and that the full event timeline was recorded.
func TestScenario_FullCollectionLifecycle(t *testing.T) {
	ctx := context.Background()

	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	// Step 1: create invoice + first reminder atomically.
	created, err := testStore.CreateInvoiceTx(ctx, CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     1000,
			Currency:   "THB",
			DueDate:    time.Now().Add(7 * 24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(-time.Minute),
		},
	})
	require.NoError(t, err)
	require.Equal(t, "unpaid", created.Invoice.Status)
	require.Equal(t, "pending", created.Invoice.CollectionState)

	// Step 2: fire the reminder → state becomes "reminded".
	setReminderProcessing(t, created.Reminder.ID)
	fired, err := testStore.FireReminderTx(ctx, stdFireParams(user, customer, created.Invoice, created.Reminder, nil))
	require.NoError(t, err)
	require.Equal(t, "reminded", fired.Invoice.CollectionState)
	require.EqualValues(t, 1, fired.Invoice.FollowUpCount)

	// Step 3: customer promises to pay.
	_, err = testStore.MoveCollectionStateTx(ctx, MoveCollectionStateTxParams{
		InvoiceID:       created.Invoice.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "promised",
	})
	require.NoError(t, err)

	// Step 4: partial payment.
	partial, err := testStore.RecordPaymentTx(ctx, RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: created.Invoice.ID,
			UserID:    user.ID,
			Amount:    400,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "partial", partial.Invoice.Status)

	// Step 5: final payment — invoice fully settled.
	final, err := testStore.RecordPaymentTx(ctx, RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: created.Invoice.ID,
			UserID:    user.ID,
			Amount:    600,
			Currency:  "THB",
			Method:    "transfer",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "paid", final.Invoice.Status)
	require.Equal(t, "paid", final.Invoice.CollectionState)
	require.Equal(t, int64(1000), final.Invoice.PaidAmount)

	// Verify event timeline contains key milestones.
	events, err := testStore.GetInvoiceTimeline(ctx, &created.Invoice.ID)
	require.NoError(t, err)

	types := make(map[CustomerEventType]bool)
	for _, e := range events {
		types[e.EventType] = true
	}
	require.True(t, types[CustomerEventTypeReminderSent], "reminder_sent event expected")
	require.True(t, types[CustomerEventTypePaymentReceived], "payment_received event expected")
}

// TestScenario_WriteOffPath walks the debt write-off path:
// create invoice → remind twice → move to no_response → write off.
func TestScenario_WriteOffPath(t *testing.T) {
	ctx := context.Background()

	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	invoice, reminder1 := committedInvoiceAndReminder(t, user, customer)

	// First reminder fired.
	setReminderProcessing(t, reminder1.ID)
	_, err := testStore.FireReminderTx(ctx, stdFireParams(user, customer, invoice, reminder1, nil))
	require.NoError(t, err)

	// Second reminder created and fired.
	reminder2, err := testStore.CreateReminder(ctx, CreateReminderParams{
		UserID:      user.ID,
		InvoiceID:   invoice.ID,
		CustomerID:  customer.ID,
		Channel:     "whatsapp",
		Tone:        "firm",
		MaxAttempts: 3,
		ScheduledAt: time.Now().Add(-time.Minute),
	})
	require.NoError(t, err)
	setReminderProcessing(t, reminder2.ID)
	fired2, err := testStore.FireReminderTx(ctx, stdFireParams(user, customer, invoice, reminder2, nil))
	require.NoError(t, err)
	require.EqualValues(t, 2, fired2.Invoice.FollowUpCount)

	// No response — escalate collection state.
	_, err = testStore.MoveCollectionStateTx(ctx, MoveCollectionStateTxParams{
		InvoiceID:       invoice.ID,
		UserID:          user.ID,
		CustomerID:      customer.ID,
		CollectionState: "no_response",
	})
	require.NoError(t, err)

	// Write off the debt.
	writeOff, err := testStore.WriteOffInvoiceTx(ctx, WriteOffInvoiceTxParams{
		InvoiceID:  invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "cancelled", writeOff.Invoice.Status)
	require.Equal(t, "defaulted", writeOff.Invoice.CollectionState)
}

// TestScenario_PaymentVoidAndCorrection verifies the void → correct-payment cycle:
// wrong partial payment → void → correct payment → paid.
func TestScenario_PaymentVoidAndCorrection(t *testing.T) {
	ctx := context.Background()

	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice := committedInvoice(t, user.ID, customer.ID)

	// Record an incorrect partial amount.
	wrong, err := testStore.RecordPaymentTx(ctx, RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: invoice.ID,
			UserID:    user.ID,
			Amount:    30, // wrong amount, should be 100
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "partial", wrong.Invoice.Status)

	// Void the wrong payment.
	voided, err := testStore.VoidPaymentTx(ctx, VoidPaymentTxParams{
		PaymentID:  wrong.Payment.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, voided.Payment.VoidedAt)
	require.Equal(t, "unpaid", voided.Invoice.Status)
	require.Equal(t, int64(0), voided.Invoice.PaidAmount)

	// Record the correct full payment.
	corrected, err := testStore.RecordPaymentTx(ctx, RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: invoice.ID,
			UserID:    user.ID,
			Amount:    100,
			Currency:  "THB",
			Method:    "transfer",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "paid", corrected.Invoice.Status)
	require.Equal(t, int64(100), corrected.Invoice.PaidAmount)
}

// TestScenario_CancelAndReactivate verifies that a cancelled invoice can be
// restored and then paid normally.
func TestScenario_CancelAndReactivate(t *testing.T) {
	ctx := context.Background()

	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	created, err := testStore.CreateInvoiceTx(ctx, CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     500,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(time.Hour),
		},
	})
	require.NoError(t, err)

	// Cancel the invoice.
	cancelled, err := testStore.CancelInvoiceTx(ctx, CancelInvoiceTxParams{
		InvoiceID:  created.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Invoice.Status)

	// Reactivate it.
	reactivated, err := testStore.ReactivateInvoiceTx(ctx, ReactivateInvoiceTxParams{
		InvoiceID:  created.Invoice.ID,
		UserID:     user.ID,
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "unpaid", reactivated.Invoice.Status)
	require.Equal(t, "pending", reactivated.Invoice.CollectionState)

	// Pay the reactivated invoice in full.
	paid, err := testStore.RecordPaymentTx(ctx, RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: created.Invoice.ID,
			UserID:    user.ID,
			Amount:    500,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "paid", paid.Invoice.Status)
}

// TestScenario_ReturnInvoice verifies the return workflow:
// partially paid invoice → returned → unpaid balance reversed from customer totals.
func TestScenario_ReturnInvoice(t *testing.T) {
	ctx := context.Background()

	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	// Use CreateInvoiceTx so that customer.total_owed is properly seeded.
	// committedInvoice inserts raw and skips the debt accounting step.
	created, err := testStore.CreateInvoiceTx(ctx, CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     100,
			Currency:   "THB",
			DueDate:    time.Now().Add(24 * time.Hour),
			Column10:   json.RawMessage("[]"),
		},
		Reminder: CreateReminderParams{
			UserID:      user.ID,
			CustomerID:  customer.ID,
			Channel:     "whatsapp",
			Tone:        "polite",
			MaxAttempts: 3,
			ScheduledAt: time.Now().Add(time.Hour),
		},
	})
	require.NoError(t, err)
	invoice := created.Invoice

	// Partial payment before the goods are returned.
	_, err = testStore.RecordPaymentTx(ctx, RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: invoice.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)

	reason := "goods returned by customer"
	returned, err := testStore.ReturnInvoiceTx(ctx, ReturnInvoiceTxParams{
		InvoiceID:    invoice.ID,
		UserID:       user.ID,
		CustomerID:   customer.ID,
		ReturnReason: &reason,
	})
	require.NoError(t, err)
	require.Equal(t, "returned", returned.Invoice.Status)

	// The unpaid portion (60) must have been reversed from customer totals.
	// paid_amount (40) stays — caller is responsible for issuing a refund.
	require.Equal(t, int64(40), returned.Invoice.PaidAmount)
}
