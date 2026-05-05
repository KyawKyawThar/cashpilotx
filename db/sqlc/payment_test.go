package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestPayment(
	t *testing.T,
	store *SQLStore,
	userID uuid.UUID,
	invoiceID uuid.UUID,
	amount int64,
) Payment {
	t.Helper()

	method := "cash"

	p, err := store.CreatePayment(context.Background(), CreatePaymentParams{
		InvoiceID: invoiceID,
		UserID:    userID,
		Amount:    amount,
		Currency:  "USD",
		Method:    method,
	})

	require.NoError(t, err)
	require.Equal(t, amount, p.Amount)
	require.Equal(t, "USD", p.Currency)

	return p
}
func TestCreatePayment_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	p := createTestPayment(t, store, user.ID, invoice.ID, 50)

	require.NotEmpty(t, p.ID)
	require.Equal(t, invoice.ID, p.InvoiceID)
	require.Equal(t, user.ID, p.UserID)
}
func TestCreatePayment_Fail_InvalidInvoice(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	_, err := store.CreatePayment(context.Background(), CreatePaymentParams{
		InvoiceID: uuid.New(), // invalid FK
		UserID:    user.ID,
		Amount:    50,
		Currency:  "USD",
		Method:    "cash",
	})

	require.Error(t, err)
}
func TestCreatePayment_Fail_WrongUserInvoice(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	other := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, err := store.RecordPaymentTx(context.Background(), RecordPaymentTxParams{
		Payment: CreatePaymentParams{
			InvoiceID: invoice.ID,
			UserID:    other.ID, // mismatch
			Amount:    50,
			Currency:  "USD",
			Method:    "cash",
		},
		CustomerID: customer.ID,
	})

	require.Error(t, err)
}
func TestGetPaymentByID_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	p := createTestPayment(t, store, user.ID, invoice.ID, 50)

	res, err := store.GetPaymentByID(context.Background(), GetPaymentByIDParams{
		ID:     p.ID,
		UserID: user.ID,
	})

	require.NoError(t, err)
	require.Equal(t, p.ID, res.ID)
}

func TestGetPaymentByID_Fail_WrongUser(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	other := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	p := createTestPayment(t, store, user.ID, invoice.ID, 50)

	_, err := store.GetPaymentByID(context.Background(), GetPaymentByIDParams{
		ID:     p.ID,
		UserID: other.ID,
	})

	require.Error(t, err)
}

func TestListPaymentsByCustomer_Filter(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	c1 := createTestCustomer(t, store, user.ID)
	c2 := createTestCustomer(t, store, user.ID)

	inv1 := createTestInvoice(t, store, user.ID, c1.ID, 100)
	inv2 := createTestInvoice(t, store, user.ID, c2.ID, 100)

	createTestPayment(t, store, user.ID, inv1.ID, 50)
	createTestPayment(t, store, user.ID, inv2.ID, 50)

	list, err := store.ListPaymentsByCustomer(context.Background(), ListPaymentsByCustomerParams{
		UserID:     user.ID,
		CustomerID: c1.ID,
		Limit:      10,
		Offset:     0,
	})

	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestListPaymentsByInvoice(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestPayment(t, store, user.ID, invoice.ID, 30)
	time.Sleep(10 * time.Millisecond)
	createTestPayment(t, store, user.ID, invoice.ID, 20)

	list, err := store.ListPaymentsByInvoice(context.Background(), ListPaymentsByInvoiceParams{
		UserID:    user.ID,
		InvoiceID: invoice.ID,
	})

	require.NoError(t, err)
	require.Len(t, list, 2)

	// ordered ASC
	require.True(t,
		list[0].CreatedAt.Before(list[1].CreatedAt) ||
			list[0].CreatedAt.Equal(list[1].CreatedAt),
	)
}
func TestSumPaymentsForInvoice(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	createTestPayment(t, store, user.ID, invoice.ID, 50)
	createTestPayment(t, store, user.ID, invoice.ID, 30)

	totalRaw, err := store.SumPaymentsForInvoice(context.Background(), invoice.ID)
	require.NoError(t, err)

	total := totalRaw // IMPORTANT

	require.Equal(t, int64(80), total)
}

func TestSumPaymentsForInvoice_Empty(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	totalRaw, err := store.SumPaymentsForInvoice(context.Background(), invoice.ID)
	require.NoError(t, err)

	total := totalRaw

	require.Equal(t, int64(0), total)
}
