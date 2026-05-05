package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestReminder(
	t *testing.T,
	store *SQLStore,
	userID uuid.UUID,
	invoiceID uuid.UUID,
	customerID uuid.UUID,
	scheduled time.Time,
) Reminder {
	t.Helper()

	r, err := store.CreateReminder(context.Background(), CreateReminderParams{
		UserID:      userID,
		InvoiceID:   invoiceID,
		CustomerID:  customerID,
		ScheduledAt: scheduled,
		Tone:        "polite",
		Channel:     "messenger",
		MaxAttempts: 3,
	})

	require.NoError(t, err)
	require.Equal(t, "pending", r.Status)

	return r
}
func TestCreateReminder_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	r := createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(time.Hour))

	require.NotEmpty(t, r.ID)
}
func TestCreateReminder_Fail_InvalidFK(t *testing.T) {
	store := newTestStore(t)

	_, err := store.CreateReminder(context.Background(), CreateReminderParams{
		UserID:      uuid.New(),
		InvoiceID:   uuid.New(),
		CustomerID:  uuid.New(),
		ScheduledAt: time.Now(),
		Tone:        "polite",
		Channel:     "whatsapp",
		MaxAttempts: 3,
	})

	require.Error(t, err)
}

func TestPickAndMarkReminders_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	list, err := store.PickAndMarkReminders(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "processing", list[0].Status)
}
func TestPickAndMarkReminders_IgnoreFuture(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(time.Hour))

	list, err := store.PickAndMarkReminders(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, list, 0)
}
func TestPickAndMarkReminders_Limit(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	for range 5 {
		invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)
		createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))
	}

	list, err := store.PickAndMarkReminders(context.Background(), 2)

	require.NoError(t, err)
	require.Len(t, list, 2)
}
func TestReminderRetryFlow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	r := createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	// first attempt → fail → goes back to pending
	list, err := store.PickAndMarkReminders(ctx, 1)
	require.NoError(t, err)
	require.Len(t, list, 1)

	errMsg := "fail"
	next := time.Now().Add(-time.Minute)

	err = store.MarkReminderFailed(ctx, MarkReminderFailedParams{
		ID:          list[0].ID,
		LastError:   &errMsg,
		NextRetryAt: &next,
	})
	require.NoError(t, err)

	// should be pending again
	rem, err := store.GetReminder(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", rem.Status)

	// retry should be picked again
	retryList, err := store.PickAndMarkReminders(ctx, 1)
	require.NoError(t, err)
	require.Len(t, retryList, 1)
	require.Equal(t, r.ID, retryList[0].ID)
}
func TestReminderRetry_NotReady(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	list, _ := store.PickAndMarkReminders(ctx, 1)

	next := time.Now().Add(time.Hour) // not ready

	err := store.MarkReminderFailed(ctx, MarkReminderFailedParams{
		ID:          list[0].ID,
		NextRetryAt: &next,
	})
	require.NoError(t, err)

	retryList, err := store.PickAndMarkReminders(ctx, 1)

	require.NoError(t, err)
	require.Len(t, retryList, 0)
}

func TestMarkReminderSent_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	list, _ := store.PickAndMarkReminders(context.Background(), 1)

	external := "ext-1"

	err := store.MarkReminderSent(context.Background(), MarkReminderSentParams{
		ID:                list[0].ID,
		ExternalMessageID: &external,
	})

	require.NoError(t, err)
}
func TestMarkReminderFailed_Retry(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	list, _ := store.PickAndMarkReminders(context.Background(), 1)

	errMsg := "network error"
	next := time.Now().Add(time.Minute)

	err := store.MarkReminderFailed(context.Background(), MarkReminderFailedParams{
		ID:          list[0].ID,
		LastError:   &errMsg,
		NextRetryAt: &next,
	})

	require.NoError(t, err)
}

func TestMarkReminderFailed_MaxAttempts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	r := createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	for i := 0; i < int(r.MaxAttempts); i++ {
		list, err := store.PickAndMarkReminders(ctx, 1)
		require.NoError(t, err)
		require.Len(t, list, 1, "expected a pickable reminder")

		errMsg := "fail"
		// 🔥 make it immediately retryable for the next loop
		next := time.Now().Add(-time.Minute)

		err = store.MarkReminderFailed(ctx, MarkReminderFailedParams{
			ID:          list[0].ID,
			LastError:   &errMsg,
			NextRetryAt: &next,
		})
		require.NoError(t, err)
	}

	final, err := store.GetReminder(ctx, r.ID)
	require.NoError(t, err)

	require.Equal(t, int32(r.MaxAttempts), final.AttemptNumber)
	require.Equal(t, "failed", final.Status) // terminal after max attempts
}

func TestCancelReminder_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	r := createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now())

	err := store.CancelReminder(context.Background(), r.ID)
	require.NoError(t, err)
}
func TestRescheduleReminder_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	r := createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now())

	newTime := time.Now().Add(2 * time.Hour)

	err := store.RescheduleReminder(context.Background(), RescheduleReminderParams{
		ID:          r.ID,
		ScheduledAt: newTime,
	})

	require.NoError(t, err)
}
func TestRescheduleReminder_Fail_InvalidState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	// 🔥 MUST be in the past so it can be picked
	r := createTestReminder(t, store, user.ID, invoice.ID, customer.ID, time.Now().Add(-time.Minute))

	// pick it → becomes processing
	list, err := store.PickAndMarkReminders(ctx, 1)
	require.NoError(t, err)
	require.Len(t, list, 1) // 🔥 guard against panic

	// move to terminal state (sent)
	external := "sent-1"
	err = store.MarkReminderSent(ctx, MarkReminderSentParams{
		ID:                list[0].ID,
		ExternalMessageID: &external,
	})
	require.NoError(t, err)

	// attempt reschedule → should NO-OP
	newTime := time.Now().Add(time.Hour)

	err = store.RescheduleReminder(ctx, RescheduleReminderParams{
		ID:          r.ID,
		ScheduledAt: newTime,
	})
	require.NoError(t, err)

	// verify NOTHING changed
	final, err := store.GetReminder(ctx, r.ID)
	require.NoError(t, err)

	require.Equal(t, "sent", final.Status)          // still sent
	require.NotEqual(t, newTime, final.ScheduledAt) // not rescheduled
}
