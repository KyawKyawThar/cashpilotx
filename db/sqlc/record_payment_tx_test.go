package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecordPaymentTx_FullPayment(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	res, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
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

	require.Equal(t, "paid", res.Invoice.Status)
	require.Equal(t, "paid", res.Invoice.CollectionState)
	require.Equal(t, int64(100), res.Payment.Amount)
}
func TestRecordPaymentTx_PartialPayment(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	res, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.NoError(t, err)

	require.Equal(t, "partial", res.Invoice.Status)
	require.NotEqual(t, "paid", res.Invoice.CollectionState)
}
func TestRecordPaymentTx_Accumulation(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, _ = testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	res, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    60,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "paid", res.Invoice.Status)
}
func TestRecordPaymentTx_InvalidAmount(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    0,
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.ErrorIs(t, err, ErrPaymentAmountInvalid)
}
func TestRecordPaymentTx_ExceedsBalance(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    200, // exceeds 100
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}
func TestRecordPaymentTx_ExactRemaining(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, _ = testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    40,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	res, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    60,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "paid", res.Invoice.Status)
}

func TestRecordPaymentTx_RollbackOnCustomerFail(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    user.ID,
			Amount:    50,
			Currency:  "THB",
			Method:    "cash",
		},
		CustomerID: uuid.New(), // invalid FK
	})

	require.Error(t, err)

	// ensure NO payment recorded
	check, err := testStore.GetInvoiceByID(context.Background(), GetInvoiceByIDParams{
		ID:     inv.ID,
		UserID: user.ID,
	})
	require.NoError(t, err)

	require.Equal(t, int64(0), check.PaidAmount)
}

func TestRecordPaymentTx_WrongUser(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	_, err := testStore.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: inv.ID,
			UserID:    other.ID,
			Amount:    50,
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.Error(t, err)
}
