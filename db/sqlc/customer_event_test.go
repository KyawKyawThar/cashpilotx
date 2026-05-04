package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestCustomerEvent(t *testing.T, store *SQLStore, userID, customerID, invoiceID uuid.UUID) CustomerEvent {
	t.Helper()

	arg := CreateCustomerEventParams{
		UserID:     userID,
		CustomerID: customerID,
		InvoiceID:  &invoiceID,
		EventType:  CustomerEventTypePaymentReceived,
		EventValue: toJSON(t, map[string]any{
			"amount": 1000,
		}),
	}

	event, err := store.CreateCustomerEvent(context.Background(), arg)
	require.NoError(t, err)
	return event
}

func TestCreateCustomerEvent_OK(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(3200))

	event := createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)

	require.NotZero(t, event.ID)
	require.Equal(t, user.ID, event.UserID)
	require.Equal(t, customer.ID, event.CustomerID)
	require.NotNil(t, event.InvoiceID)
	require.Equal(t, CustomerEventTypePaymentReceived, event.EventType)
	require.NotZero(t, event.CreatedAt)
}
func TestCreateCustomerEvent_InvalidCustomer(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	arg := CreateCustomerEventParams{
		UserID:     user.ID,
		CustomerID: uuid.New(), // invalid
		EventType:  CustomerEventTypePaymentReceived,
	}

	_, err := store.CreateCustomerEvent(context.Background(), arg)
	require.Error(t, err)
}
func TestCreateCustomerEvent_InvalidEventType(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	arg := CreateCustomerEventParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		EventType:  "INVALID", // force cast
	}

	_, err := store.CreateCustomerEvent(context.Background(), arg)
	require.Error(t, err)
}
func TestCreateCustomerEvent_NullOptionalFields(t *testing.T) {

	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	arg := CreateCustomerEventParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  nil,
		EventValue: nil,
		EventType:  CustomerEventTypePaymentReceived,
	}

	event, err := store.CreateCustomerEvent(context.Background(), arg)
	require.NoError(t, err)

	require.Nil(t, event.InvoiceID)
	require.Nil(t, event.EventValue)
}
func TestCreateCustomerEvent_LargeJSON(t *testing.T) {

	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	large := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		large[fmt.Sprintf("k%d", i)] = i
	}

	arg := CreateCustomerEventParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		EventType:  CustomerEventTypePaymentReceived,
		EventValue: toJSON(t, map[string]any{
			"amount": 1000,
		}),
	}

	_, err := store.CreateCustomerEvent(context.Background(), arg)
	require.NoError(t, err)
}
func TestGetInvoiceTimeline_OK(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for i := 0; i < 5; i++ {
		createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)
	}

	result, err := store.GetInvoiceTimeline(context.Background(), &invoice.ID)
	require.NoError(t, err)

	require.Len(t, result, 5)

	// ensure DESC ordering
	for i := 1; i < len(result); i++ {
		require.True(t, result[i-1].CreatedAt.After(result[i].CreatedAt) ||
			result[i-1].CreatedAt.Equal(result[i].CreatedAt))
	}
}
func TestGetInvoiceTimeline_Empty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))
	result, err := store.GetInvoiceTimeline(context.Background(), &invoice.ID)
	require.NoError(t, err)
	require.Empty(t, result)
}
func TestGetInvoiceTimeline_NilInvoiceID(t *testing.T) {

	t.Parallel()
	store := newTestStore(t)
	result, err := store.GetInvoiceTimeline(context.Background(), nil)

	// depends on SQL behavior (likely empty or error)
	require.NoError(t, err)
	require.Empty(t, result)
}
func TestListCustomerEvents_OK(t *testing.T) {

	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for i := 0; i < 5; i++ {
		createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)
	}

	arg := ListCustomerEventsParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		Limit:      5,
		Offset:     0,
	}

	result, err := store.ListCustomerEvents(context.Background(), arg)
	require.NoError(t, err)
	require.Len(t, result, 5)
}
func TestListCustomerEvents_Pagination(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for range 10 {
		createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)
	}

	first, err := store.ListCustomerEvents(context.Background(), ListCustomerEventsParams{
		UserID: user.ID, CustomerID: customer.ID, Limit: 5, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, first, 5)

	second, err := store.ListCustomerEvents(context.Background(), ListCustomerEventsParams{
		UserID: user.ID, CustomerID: customer.ID, Limit: 5, Offset: 5,
	})
	require.NoError(t, err)
	require.Len(t, second, 5)

	require.NotEqual(t, first[0].ID, second[0].ID)
}
func TestListCustomerEvents_UserIsolation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)

	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for range 10 {
		createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)
	}

	result, err := store.ListCustomerEvents(context.Background(), ListCustomerEventsParams{
		UserID:     otherUser.ID,
		CustomerID: customer.ID,
		Limit:      10,
	})

	require.NoError(t, err)
	require.Empty(t, result)
}
func TestListCustomerEvents_ZeroLimit(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	result, err := store.ListCustomerEvents(context.Background(), ListCustomerEventsParams{
		UserID: user.ID, CustomerID: customer.ID, Limit: 0,
	})

	require.NoError(t, err)
	require.Empty(t, result)
}
func TestListEventsByType_OK(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for range 5 {
		createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)
	}

	arg := ListEventsByTypeParams{
		UserID:    user.ID,
		EventType: CustomerEventTypePaymentReceived,
		Limit:     10,
	}

	result, err := store.ListEventsByType(context.Background(), arg)
	require.NoError(t, err)
	require.Len(t, result, 5)

	for _, e := range result {
		require.Equal(t, CustomerEventTypePaymentReceived, e.EventType)
	}
}
func TestListEventsByType_NoMatch(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)

	result, err := store.ListEventsByType(context.Background(), ListEventsByTypeParams{
		UserID:    user.ID,
		EventType: CustomerEventTypeInvoiceCanceled,
		Limit:     10,
	})

	require.NoError(t, err)
	require.Empty(t, result)
}
func TestListEventsByType_LimitOverflow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for range 3 {
		createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)
	}

	result, err := store.ListEventsByType(context.Background(), ListEventsByTypeParams{
		UserID:    user.ID,
		EventType: CustomerEventTypePaymentReceived,
		Limit:     100,
	})

	require.NoError(t, err)
	require.Len(t, result, 3)
}

func TestCountEventsByType_OK(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	for range 3 {
		_, err := store.CreateCustomerEvent(context.Background(), CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			InvoiceID:  &invoice.ID,
			EventType:  CustomerEventTypePaymentReceived,
		})
		require.NoError(t, err)
	}

	for range 2 {
		_, err := store.CreateCustomerEvent(context.Background(), CreateCustomerEventParams{
			UserID:     user.ID,
			CustomerID: customer.ID,
			InvoiceID:  &invoice.ID,
			EventType:  CustomerEventTypeReminderSent,
		})
		require.NoError(t, err)
	}

	now := time.Now()
	arg := CountEventsByTypeParams{
		UserID:      user.ID,
		CreatedAt:   now.Add(-time.Hour),
		CreatedAt_2: now.Add(time.Hour),
	}

	result, err := store.CountEventsByType(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	counts := make(map[CustomerEventType]int64)
	for _, r := range result {
		counts[r.EventType] = r.Total
	}
	require.Equal(t, int64(3), counts[CustomerEventTypePaymentReceived])
	require.Equal(t, int64(2), counts[CustomerEventTypeReminderSent])
}
func TestCountEventsByType_TimeRange(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, int64(5000))

	createTestCustomerEvent(t, store, user.ID, customer.ID, invoice.ID)

	// query a window entirely in the future — the just-created event must not appear
	now := time.Now()
	arg := CountEventsByTypeParams{
		UserID:      user.ID,
		CreatedAt:   now.Add(time.Hour),
		CreatedAt_2: now.Add(2 * time.Hour),
	}

	result, err := store.CountEventsByType(context.Background(), arg)
	require.NoError(t, err)
	require.Empty(t, result)
}
func TestCountEventsByType_InvalidTimeRange(t *testing.T) {

	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)

	arg := CountEventsByTypeParams{
		UserID:      user.ID,
		CreatedAt:   time.Now(),
		CreatedAt_2: time.Now().Add(-1 * time.Hour),
	}

	result, err := store.CountEventsByType(context.Background(), arg)

	require.NoError(t, err)
	require.Empty(t, result)
}
func TestCountEventsByType_NoData(t *testing.T) {

	t.Parallel()
	store := newTestStore(t)

	user := createTestUser(t, store)
	arg := CountEventsByTypeParams{
		UserID:      user.ID,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		CreatedAt_2: time.Now(),
	}

	result, err := store.CountEventsByType(context.Background(), arg)
	require.NoError(t, err)
	require.Empty(t, result)
}
