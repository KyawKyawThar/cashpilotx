package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// -----------------------------
// Helper
// -----------------------------

func createTestInvoice(t *testing.T, store *SQLStore, userID, customerID uuid.UUID, amount int64) Invoice {
	t.Helper()

	desc := "test invoice"
	source := "manual"

	inv, err := store.CreateInvoice(context.Background(), CreateInvoiceParams{
		UserID:      userID,
		CustomerID:  customerID,
		Amount:      amount,
		Currency:    "THB",
		Description: &desc,
		DueDate:     time.Now().Add(24 * time.Hour),
		Source:      &source,
		Column10:    json.RawMessage("[]"),
	})
	require.NoError(t, err)

	require.Equal(t, amount, inv.Amount)
	require.Equal(t, int64(0), inv.PaidAmount)
	require.Equal(t, "THB", inv.Currency)
	require.Equal(t, "unpaid", inv.Status)

	return inv
}
func TestCreateInvoice_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	require.NotEmpty(t, inv.ID)
	require.Equal(t, user.ID, inv.UserID)
	require.Equal(t, customer.ID, inv.CustomerID)
}

func TestCreateInvoice_Fail_InvalidForeignKey(t *testing.T) {
	store := newTestStore(t)

	invalidUser := uuid.New()
	invalidCustomer := uuid.New()

	_, err := store.CreateInvoice(context.Background(), CreateInvoiceParams{
		UserID:     invalidUser,
		CustomerID: invalidCustomer,
		Amount:     100,
		Currency:   "THB",
		DueDate:    time.Now(),
	})

	require.Error(t, err)
}
func TestGetInvoiceByID_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	result, err := store.GetInvoiceByID(context.Background(), GetInvoiceByIDParams{
		ID:     inv.ID,
		UserID: user.ID,
	})

	require.NoError(t, err)
	require.Equal(t, inv.ID, result.ID)
}
func TestGetInvoiceByID_Fail_WrongUser(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)
	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, err := store.GetInvoiceByID(context.Background(), GetInvoiceByIDParams{
		ID:     inv.ID,
		UserID: otherUser.ID,
	})

	require.Error(t, err)
}
func TestAddPayment_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	updated, err := store.AddPayment(context.Background(), AddPaymentParams{
		ID:         inv.ID,
		UserID:     user.ID,
		PaidAmount: 40,
	})

	require.NoError(t, err)
	require.Equal(t, int64(40), updated.PaidAmount)
}
func TestAddPayment_Fail_WrongUser(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)
	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, err := store.AddPayment(context.Background(), AddPaymentParams{
		ID:         inv.ID,
		UserID:     otherUser.ID,
		PaidAmount: 10,
	})

	require.Error(t, err)
}
func TestAddPayment_Edge_Overpay(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, err := store.AddPayment(context.Background(), AddPaymentParams{
		ID:         inv.ID,
		UserID:     user.ID,
		PaidAmount: 200,
	})

	require.Error(t, err)

	// Optional: assert specific constraint
	require.Contains(t, err.Error(), "invoice_no_overpay")
}
func TestRecomputeInvoiceStatus_Unpaid(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	res, err := store.RecomputeInvoiceStatus(context.Background(), RecomputeInvoiceStatusParams{
		ID:     inv.ID,
		UserID: user.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "unpaid", res.Status)
}
func TestRecomputeInvoiceStatus_Partial(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, _ = store.CreatePayment(context.Background(), CreatePaymentParams{
		InvoiceID: inv.ID,
		UserID:    user.ID,
		Amount:    40,
		Currency:  "THB",
		Method:    "transfer",
	})

	res, err := store.RecomputeInvoiceStatus(context.Background(), RecomputeInvoiceStatusParams{
		ID:     inv.ID,
		UserID: user.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "partial", res.Status)
}
func TestRecomputeInvoiceStatus_Paid(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, _ = store.CreatePayment(context.Background(), CreatePaymentParams{
		InvoiceID: inv.ID,
		UserID:    user.ID,
		Amount:    100,
		Currency:  "THB",
		Method:    "transfer",
	})

	res, err := store.RecomputeInvoiceStatus(context.Background(), RecomputeInvoiceStatusParams{
		ID:     inv.ID,
		UserID: user.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "paid", res.Status)
	require.NotNil(t, res.PaidAt)
}
func TestListInvoices_Pagination(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	for i := 0; i < 5; i++ {
		createTestInvoice(t, store, user.ID, customer.ID, 100)
	}

	list, err := store.ListInvoices(context.Background(), ListInvoicesParams{
		UserID: user.ID,
		Limit:  2,
		Offset: 0,
	})

	require.NoError(t, err)
	require.Len(t, list, 2)
}
func TestListInvoicesByCustomer_Filter(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	c1 := createTestCustomer(t, store, user.ID)
	c2 := createTestCustomer(t, store, user.ID)

	createTestInvoice(t, store, user.ID, c1.ID, 100)
	createTestInvoice(t, store, user.ID, c2.ID, 100)

	list, err := store.ListInvoicesByCustomer(context.Background(), ListInvoicesByCustomerParams{
		UserID:     user.ID,
		CustomerID: c1.ID,
		Limit:      10,
		Offset:     0,
	})

	require.NoError(t, err)
	require.Len(t, list, 1)
}
func TestListOverdueInvoices(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	_, err := store.CreateInvoice(context.Background(), CreateInvoiceParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		Amount:     100,
		Currency:   "THB",
		DueDate:    time.Now().UTC().AddDate(0, 0, -1), // past — use UTC so stored date is unambiguously before CURRENT_DATE
		Column10:   json.RawMessage("[]"),
	})

	require.NoError(t, err)

	list, err := store.ListOverdueInvoices(context.Background(), ListOverdueInvoicesParams{
		UserID: user.ID,
		Limit:  10,
		Offset: 0,
	})

	require.NoError(t, err)
	require.Len(t, list, 1)
}
func TestCancelInvoice_Success(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	res, err := store.CancelInvoice(context.Background(), CancelInvoiceParams{
		ID:     inv.ID,
		UserID: user.ID,
	})

	require.NoError(t, err)
	require.Equal(t, "cancelled", res.Status) //nolint:misspell // DB constraint uses British spelling
}

func TestCancelInvoice_Fail_WrongUser(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	other := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, err := store.CancelInvoice(context.Background(), CancelInvoiceParams{
		ID:     inv.ID,
		UserID: other.ID,
	})

	require.Error(t, err)
}
func TestListInvoicesByStatus_Filter(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	// unpaid
	createTestInvoice(t, store, user.ID, customer.ID, 100)

	// paid
	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)
	_, _ = store.CreatePayment(context.Background(), CreatePaymentParams{
		InvoiceID: inv.ID, UserID: user.ID, Amount: 100, Currency: "THB", Method: "transfer",
	})
	_, _ = store.RecomputeInvoiceStatus(context.Background(), RecomputeInvoiceStatusParams{
		ID: inv.ID, UserID: user.ID,
	})

	list, err := store.ListInvoicesByStatus(context.Background(), ListInvoicesByStatusParams{
		UserID: user.ID,
		Status: "paid",
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "paid", list[0].Status)
}
func TestSetInvoiceContacted(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	res, err := store.SetInvoiceContacted(context.Background(), SetInvoiceContactedParams{
		ID:              inv.ID,
		UserID:          user.ID,
		CollectionState: "reminded",
	})

	require.NoError(t, err)
	require.Equal(t, "reminded", res.CollectionState)
	require.NotNil(t, res.LastContactedAt)
	require.Equal(t, int32(1), res.FollowUpCount)
}
func TestSetInvoiceReplied(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	res, err := store.SetInvoiceReplied(context.Background(), SetInvoiceRepliedParams{
		ID:              inv.ID,
		UserID:          user.ID,
		CollectionState: "negotiating",
	})

	require.NoError(t, err)
	require.Equal(t, "negotiating", res.CollectionState)
	require.NotNil(t, res.LastResponseAt)
}
func TestSetInvoicePromised(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	promised := time.Now().Add(48 * time.Hour)

	res, err := store.SetInvoicePromised(context.Background(), SetInvoicePromisedParams{
		ID:         inv.ID,
		UserID:     user.ID,
		PromisedAt: &promised,
	})

	require.NoError(t, err)
	require.Equal(t, "promised", res.CollectionState)
	require.NotNil(t, res.LastResponseAt)
	require.NotNil(t, res.PromisedAt)
}
func TestUpdateInvoiceCollectionState(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	res, err := store.UpdateInvoiceCollectionState(context.Background(), UpdateInvoiceCollectionStateParams{
		ID:              inv.ID,
		UserID:          user.ID,
		CollectionState: "defaulted",
	})

	require.NoError(t, err)
	require.Equal(t, "defaulted", res.CollectionState)
}
func TestUpdateInvoiceConfidence(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	score := 0.85
	factors := json.RawMessage(`[{"label":"late","signal":"high"}]`)
	text := "Likely to pay"
	timeframe := "3 days"

	rowsAffected, err := store.UpdateInvoiceConfidence(context.Background(), UpdateInvoiceConfidenceParams{
		ID:                inv.ID,
		UserID:            user.ID,
		ConfidenceScore:   &score,
		ConfidenceFactors: &factors,
		OutcomeText:       &text,
		OutcomeTimeframe:  &timeframe,
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, rowsAffected)
}
func TestGetRecoveryQueue_OrderAndJoin(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	list, err := store.GetRecoveryQueue(context.Background(), GetRecoveryQueueParams{
		UserID: user.ID,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, list, 1)

	require.Equal(t, inv.ID, list[0].ID)
	require.NotEmpty(t, list[0].CustomerName)
}
func TestGetRecoveryStats(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	createTestInvoice(t, store, user.ID, customer.ID, 100)
	createTestInvoice(t, store, user.ID, customer.ID, 200)

	stats, err := store.GetRecoveryStats(context.Background(), user.ID)

	require.NoError(t, err)
	require.Greater(t, stats.TotalRecoverable, int64(0))
}
func TestListUnscored(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	list, err := store.ListUnscored(context.Background(), 10)

	require.NoError(t, err)
	require.NotEmpty(t, list)

	found := false
	for _, v := range list {
		if v.ID == inv.ID {
			found = true
		}
	}
	require.True(t, found)
}
func TestCancelInvoice_Idempotent(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	inv := createTestInvoice(t, store, user.ID, customer.ID, 100)

	_, _ = store.CancelInvoice(context.Background(), CancelInvoiceParams{
		ID: inv.ID, UserID: user.ID,
	})

	_, err := store.CancelInvoice(context.Background(), CancelInvoiceParams{
		ID: inv.ID, UserID: user.ID,
	})

	require.Error(t, err) // because WHERE status != 'cancelled'
}
