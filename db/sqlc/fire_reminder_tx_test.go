package db

import (
	"cashpilotx/utils"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// committedInvoiceAndReminder creates a real committed invoice+reminder via testStore.
// The reminder starts in 'pending' status; call setReminderProcessing before FireReminderTx.
func committedInvoiceAndReminder(t *testing.T, user User, customer Customer) (Invoice, Reminder) {
	t.Helper()
	result, err := testStore.CreateInvoiceTx(context.Background(), CreateInvoiceTxParams{
		Invoice: CreateInvoiceParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			Amount:     1000,
			Currency:   "USD",
			DueDate:    time.Now().Add(24 * time.Hour),
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
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM customer_events WHERE invoice_id = $1", result.Invoice.ID)
		_, _ = testDB.Exec(context.Background(), "DELETE FROM messages WHERE invoice_id = $1", result.Invoice.ID)
		_, _ = testDB.Exec(context.Background(), "DELETE FROM reminders WHERE invoice_id = $1", result.Invoice.ID)
		_, _ = testDB.Exec(context.Background(), "DELETE FROM invoices WHERE id = $1", result.Invoice.ID)
	})
	return result.Invoice, result.Reminder
}

// setReminderProcessing moves a reminder to 'processing' state directly, bypassing
// PickAndMarkReminders (which is time-gated and requires the worker queue loop).
func setReminderProcessing(t *testing.T, id uuid.UUID) {
	t.Helper()
	_, err := testDB.Exec(context.Background(),
		"UPDATE reminders SET status = 'processing' WHERE id = $1", id)
	require.NoError(t, err)
}

// stdFireParams returns a FireReminderTxParams with sensible defaults for the given fixtures.
func stdFireParams(user User, customer Customer, invoice Invoice, reminder Reminder, next *CreateReminderParams) FireReminderTxParams {
	invID := invoice.ID
	extID := "ext-" + utils.RandomString(8)
	return FireReminderTxParams{
		ReminderUpdate: MarkReminderSentParams{
			ID:                reminder.ID,
			ExternalMessageID: &extID,
		},
		Message: CreateMessageParams{
			UserID:     user.ID,
			InvoiceID:  &invID,
			CustomerID: customer.ID,
			Direction:  "outbound",
			Channel:    "whatsapp",
			Content:    "Hi, please settle your invoice of USD 1000.",
			Tone:       "polite",
			Column8:    "sent",
			Column9:    "ai",
			Column10:   json.RawMessage("{}"),
		},
		NextReminder: next,
		InvoiceID:    invoice.ID,
		UserID:       user.ID,
		CustomerID:   customer.ID,
	}
}

func TestFireReminderTx_Success_NoFollowUp(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice, reminder := committedInvoiceAndReminder(t, user, customer)
	setReminderProcessing(t, reminder.ID)

	result, err := testStore.FireReminderTx(context.Background(), stdFireParams(user, customer, invoice, reminder, nil))
	require.NoError(t, err)

	// Invoice must be advanced to 'reminded' with follow_up_count incremented.
	require.Equal(t, "reminded", result.Invoice.CollectionState)
	require.EqualValues(t, 1, result.Invoice.FollowUpCount)
	require.NotNil(t, result.Invoice.LastContactedAt)

	// Outbound message must be recorded.
	require.NotZero(t, result.Message.ID)
	require.Equal(t, user.ID, result.Message.UserID)
	require.Equal(t, customer.ID, result.Message.CustomerID)
	require.Equal(t, invoice.ID, *result.Message.InvoiceID)
	require.Equal(t, "whatsapp", result.Message.Channel)

	// No follow-up reminder when nil is passed.
	require.Nil(t, result.NextReminder)

	// Reminder must be 'sent' with SentAt and ExternalMessageID set in the DB.
	dbReminder, err := testStore.GetReminder(context.Background(), reminder.ID)
	require.NoError(t, err)
	require.Equal(t, "sent", dbReminder.Status)
	require.NotNil(t, dbReminder.SentAt)
	require.NotNil(t, dbReminder.ExternalMessageID)

	// 'reminder_sent' event must appear in the invoice timeline.
	events, err := testStore.GetInvoiceTimeline(context.Background(), &invoice.ID)
	require.NoError(t, err)
	var found bool
	for _, e := range events {
		if e.EventType == CustomerEventTypeReminderSent {
			found = true
			break
		}
	}
	require.True(t, found, "reminder_sent event must be recorded in the invoice timeline")
}

func TestFireReminderTx_Success_WithNextReminder(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice, reminder := committedInvoiceAndReminder(t, user, customer)
	setReminderProcessing(t, reminder.ID)

	nextAt := time.Now().Add(48 * time.Hour)
	next := &CreateReminderParams{
		UserID:      user.ID,
		InvoiceID:   invoice.ID,
		CustomerID:  customer.ID,
		Channel:     "whatsapp",
		Tone:        "firm",
		MaxAttempts: 3,
		ScheduledAt: nextAt,
	}

	result, err := testStore.FireReminderTx(context.Background(), stdFireParams(user, customer, invoice, reminder, next))
	require.NoError(t, err)

	// Follow-up reminder must be created with 'pending' status.
	require.NotNil(t, result.NextReminder)
	require.NotZero(t, result.NextReminder.ID)
	require.Equal(t, "pending", result.NextReminder.Status)
	require.Equal(t, "firm", result.NextReminder.Tone)
	require.Equal(t, invoice.ID, result.NextReminder.InvoiceID)
	require.WithinDuration(t, nextAt, result.NextReminder.ScheduledAt, time.Second)
}

// TestFireReminderTx_FollowUpCountIncrements verifies that successive fires on the same
// invoice each advance follow_up_count by one.
func TestFireReminderTx_FollowUpCountIncrements(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice, reminder1 := committedInvoiceAndReminder(t, user, customer)
	setReminderProcessing(t, reminder1.ID)

	res1, err := testStore.FireReminderTx(context.Background(), stdFireParams(user, customer, invoice, reminder1, nil))
	require.NoError(t, err)
	require.EqualValues(t, 1, res1.Invoice.FollowUpCount)

	// Create and fire a second reminder for the same invoice.
	reminder2, err := testStore.CreateReminder(context.Background(), CreateReminderParams{
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

	res2, err := testStore.FireReminderTx(context.Background(), stdFireParams(user, customer, invoice, reminder2, nil))
	require.NoError(t, err)
	require.EqualValues(t, 2, res2.Invoice.FollowUpCount)
}

// TestFireReminderTx_ReminderNotProcessing_TxSucceedsButReminderUnchanged documents a
// known behavioral edge case: MarkReminderSent uses WHERE status = 'processing', so
// passing a reminder that is still 'pending' is a silent no-op — the transaction still
// commits, but the reminder status is not updated. Callers must ensure the reminder is
// in 'processing' before calling FireReminderTx.
func TestFireReminderTx_ReminderNotProcessing_TxSucceedsButReminderUnchanged(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice, reminder := committedInvoiceAndReminder(t, user, customer)
	// Deliberately skip setReminderProcessing — reminder stays 'pending'.

	result, err := testStore.FireReminderTx(context.Background(), stdFireParams(user, customer, invoice, reminder, nil))
	require.NoError(t, err, "FireReminderTx must not error when reminder is not in processing state")

	// Invoice state is still advanced — SetInvoiceContacted runs regardless.
	require.Equal(t, "reminded", result.Invoice.CollectionState)
	require.EqualValues(t, 1, result.Invoice.FollowUpCount)

	// Reminder must remain 'pending'; MarkReminderSent did not match the row.
	dbReminder, err := testStore.GetReminder(context.Background(), reminder.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", dbReminder.Status, "reminder must stay 'pending' when not in processing state")
	require.Nil(t, dbReminder.SentAt)
}

// TestFireReminderTx_UnknownInvoice_RollsBack verifies that a non-existent InvoiceID
// causes SetInvoiceContacted to return ErrNoRows, rolling back the entire transaction.
func TestFireReminderTx_UnknownInvoice_RollsBack(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	bogusInvoiceID := uuid.New()
	extID := "ext-bogus"

	_, err := testStore.FireReminderTx(context.Background(), FireReminderTxParams{
		ReminderUpdate: MarkReminderSentParams{
			ID:                uuid.New(),
			ExternalMessageID: &extID,
		},
		Message: CreateMessageParams{
			UserID:     user.ID,
			InvoiceID:  nil, // nil avoids FK failure in messages before SetInvoiceContacted runs
			CustomerID: customer.ID,
			Direction:  "outbound",
			Channel:    "whatsapp",
			Content:    "test",
			Tone:       "polite",
			Column8:    "sent",
			Column9:    "ai",
			Column10:   json.RawMessage("{}"),
		},
		NextReminder: nil,
		InvoiceID:    bogusInvoiceID,
		UserID:       user.ID,
		CustomerID:   customer.ID,
	})
	require.Error(t, err, "SetInvoiceContacted must fail for a non-existent invoice and roll back the tx")
}

// TestFireReminderTx_WrongUserID_RollsBack verifies that passing a UserID that does not
// own the invoice causes SetInvoiceContacted (WHERE user_id = $2) to return ErrNoRows,
// rolling back all steps including MarkReminderSent.
func TestFireReminderTx_WrongUserID_RollsBack(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice, reminder := committedInvoiceAndReminder(t, user, customer)
	setReminderProcessing(t, reminder.ID)

	intruder := committedUser(t)
	invID := invoice.ID

	_, err := testStore.FireReminderTx(context.Background(), FireReminderTxParams{
		ReminderUpdate: MarkReminderSentParams{
			ID:                reminder.ID,
			ExternalMessageID: nil,
		},
		Message: CreateMessageParams{
			UserID:     intruder.ID,
			InvoiceID:  &invID,
			CustomerID: customer.ID,
			Direction:  "outbound",
			Channel:    "whatsapp",
			Content:    "test",
			Tone:       "polite",
			Column8:    "sent",
			Column9:    "ai",
			Column10:   json.RawMessage("{}"),
		},
		NextReminder: nil,
		InvoiceID:    invoice.ID,
		UserID:       intruder.ID, // does not own the invoice
		CustomerID:   customer.ID,
	})
	require.Error(t, err, "SetInvoiceContacted must fail when UserID does not own the invoice")

	// Rollback must revert MarkReminderSent — reminder returns to 'processing'.
	dbReminder, err := testStore.GetReminder(context.Background(), reminder.ID)
	require.NoError(t, err)
	require.Equal(t, "processing", dbReminder.Status, "rollback must revert MarkReminderSent")
}

// TestFireReminderTx_InvalidNextReminder_RollsBack verifies that a NextReminder with a
// non-existent InvoiceID fails the FK constraint in CreateReminder, rolling back all
// prior steps atomically — including MarkReminderSent and SetInvoiceContacted.
func TestFireReminderTx_InvalidNextReminder_RollsBack(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	invoice, reminder := committedInvoiceAndReminder(t, user, customer)
	setReminderProcessing(t, reminder.ID)

	next := &CreateReminderParams{
		UserID:      user.ID,
		InvoiceID:   uuid.New(), // does not exist — FK violation on reminders.invoice_id
		CustomerID:  customer.ID,
		Channel:     "whatsapp",
		Tone:        "firm",
		MaxAttempts: 3,
		ScheduledAt: time.Now().Add(48 * time.Hour),
	}

	_, err := testStore.FireReminderTx(context.Background(), stdFireParams(user, customer, invoice, reminder, next))
	require.Error(t, err, "CreateReminder with a non-existent InvoiceID must fail and roll back the tx")

	// All prior steps must be rolled back; reminder returns to 'processing'.
	dbReminder, err := testStore.GetReminder(context.Background(), reminder.ID)
	require.NoError(t, err)
	require.Equal(t, "processing", dbReminder.Status, "rollback must revert MarkReminderSent")

	// Invoice follow_up_count must not have been incremented.
	dbInvoice, err := testStore.GetInvoiceByID(context.Background(), GetInvoiceByIDParams{
		ID:     invoice.ID,
		UserID: user.ID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, dbInvoice.FollowUpCount, "rollback must revert SetInvoiceContacted")
}
