package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRefreshDailyStatsTx_Success(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	today := time.Now()

	// create invoice
	inv := committedInvoice(t, user.ID, customer.ID)

	// payment today
	_, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    100,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)

	// simulate message sent today
	msg := createTestMessage(t, testStore.(*SQLStore), user.ID, customer.ID, &inv.ID)
	_, err = testStore.MarkMessageSent(context.Background(), MarkMessageSentParams{
		ID: msg.ID,
	})
	require.NoError(t, err)

	res, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   today,
	})
	require.NoError(t, err)

	require.Equal(t, int64(100), res.Stats.TotalCollected)
	require.GreaterOrEqual(t, res.Stats.MessagesSent, int32(1))
}

func TestRefreshDailyStatsTx_Empty(t *testing.T) {
	user := committedUser(t)

	res, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   time.Now(),
	})
	require.NoError(t, err)

	require.Equal(t, int64(0), res.Stats.TotalCollected)
	require.Equal(t, int32(0), res.Stats.MessagesSent)
	require.Equal(t, int32(0), res.Stats.InvoicesPaid)
}
func TestRefreshDailyStatsTx_DateIsolation(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	yesterday := time.Now().AddDate(0, 0, -1)

	inv := committedInvoice(t, user.ID, customer.ID)

	// payment inserted now (created_at = today)
	_, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    100,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})
	require.NoError(t, err)

	// querying yesterday must not include today's payment
	res, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   yesterday,
	})
	require.NoError(t, err)

	require.Equal(t, int64(0), res.Stats.TotalCollected)
}
func TestRefreshDailyStatsTx_MultipleRecords(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)

	today := time.Now()

	inv1 := committedInvoice(t, user.ID, customer.ID)
	inv2 := committedInvoice(t, user.ID, customer.ID)

	_, _ = testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv1.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	_, _ = testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv2.ID,
			UserID:    user.ID,
			Amount:    60,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	res, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   today,
	})
	require.NoError(t, err)

	require.Equal(t, int64(100), res.Stats.TotalCollected)
}

func TestRefreshDailyStatsTx_Idempotent(t *testing.T) {
	user := committedUser(t)

	date := time.Now()

	first, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   date,
	})
	require.NoError(t, err)

	second, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   date,
	})
	require.NoError(t, err)

	require.Equal(t, first.Stats.TotalCollected, second.Stats.TotalCollected)
	require.Equal(t, first.Stats.MessagesSent, second.Stats.MessagesSent)
}

func TestRefreshDailyStatsTx_UserIsolation(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)

	today := time.Now()

	inv := committedInvoice(t, user.ID, customer.ID)

	_, _ = testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    100,
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	res, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: other.ID,
		Date:   today,
	})
	require.NoError(t, err)

	require.Equal(t, int64(0), res.Stats.TotalCollected)
}

func TestRefreshDailyStatsTx_Rollback(t *testing.T) {
	user := committedUser(t)

	// invalid date shouldn't fail, but let's simulate edge case
	_, err := testStore.RefreshDailyStatsTx(context.Background(), RefreshDailyStatsTxParams{
		UserID: user.ID,
		Date:   time.Time{}, // zero date
	})

	// depends on DB handling, but should not panic
	require.NoError(t, err)
}
