package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestExchangeRate(
	t *testing.T,
	store *SQLStore,
	userID uuid.UUID,
	from, to string,
	rate string,
	validFrom time.Time,
) ExchangeRate {
	t.Helper()

	arg := CreateExchangeRateParams{
		UserID:       userID,
		FromCurrency: from,
		ToCurrency:   to,
		Rate:         rate,
		Source:       RateSourceManual,
		ValidFrom:    validFrom,
	}

	res, err := store.CreateExchangeRate(context.Background(), arg)
	require.NoError(t, err)
	return res
}

func TestCreateExchangeRate_OK(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	now := time.Now()

	rate := createTestExchangeRate(t, store, user.ID, "THB", "USD", "35.5", now)

	require.NotZero(t, rate.ID)
	require.Equal(t, "THB", rate.FromCurrency)
	require.Equal(t, "USD", rate.ToCurrency)
	require.Nil(t, rate.ValidTo)
}
func TestGetActiveExchangeRate_OK(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	now := time.Now()
	expected := createTestExchangeRate(t, store, user.ID, "THB", "USD", "35", now)

	result, err := store.GetActiveExchangeRate(context.Background(), GetActiveExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
	})

	require.NoError(t, err)
	require.Equal(t, expected.ID, result.ID)
}

func TestGetActiveExchangeRate_NotFound(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	_, err := store.GetActiveExchangeRate(context.Background(), GetActiveExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
	})

	require.Error(t, err)
}
func TestCloseCurrentExchangeRate(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	now := time.Now()
	createTestExchangeRate(t, store, user.ID, "THB", "USD", "35", now)

	closeTime := now.Add(time.Hour)

	err := store.CloseCurrentExchangeRate(context.Background(), CloseCurrentExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		ValidTo:      &closeTime,
	})
	require.NoError(t, err)

	_, err = store.GetActiveExchangeRate(context.Background(), GetActiveExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
	})
	require.Error(t, err)
}
func TestCloseExchangeRateByID_Idempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	rate := createTestExchangeRate(t, store, user.ID, "THB", "USD", "35", time.Now().Add(-time.Second))

	err := store.CloseExchangeRateByID(context.Background(), rate.ID)
	require.NoError(t, err)

	// second close → should not fail
	err = store.CloseExchangeRateByID(context.Background(), rate.ID)
	require.NoError(t, err)
}
func TestGetExchangeRateAtTime_History(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	base := time.Now()

	// rate 1
	r1 := createTestExchangeRate(t, store, user.ID, "THB", "USD", "34", base)

	// close r1
	closeTime := base.Add(1 * time.Hour)
	err := store.CloseCurrentExchangeRate(context.Background(), CloseCurrentExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		ValidTo:      &closeTime,
	})
	require.NoError(t, err)

	// rate 2
	r2 := createTestExchangeRate(t, store, user.ID, "THB", "USD", "36", closeTime)

	// query before switch
	res1, err := store.GetExchangeRateAtTime(context.Background(), GetExchangeRateAtTimeParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		ValidFrom:    base.Add(30 * time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, r1.ID, res1.ID)

	// query after switch
	res2, err := store.GetExchangeRateAtTime(context.Background(), GetExchangeRateAtTimeParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		ValidFrom:    closeTime.Add(30 * time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, r2.ID, res2.ID)
}
func TestGetExchangeRateAtTime_NoMatch(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	_, err := store.GetExchangeRateAtTime(context.Background(), GetExchangeRateAtTimeParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		ValidFrom:    time.Now(),
	})

	require.Error(t, err)
}
func TestListExchangeRates_Pagination(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	now := time.Now().Truncate(time.Second) // Avoid DB precision issues

	for i := range 10 {
		// Create a rate that is currently active
		createTestExchangeRate(t, store, user.ID, "THB", "USD", "35", now.Add(time.Duration(i)*time.Minute))

		// If this isn't the last rate we're creating, close it.
		// The close time will be the start time of the next rate.
		if i < 9 {
			closeTime := now.Add(time.Duration(i+1) * time.Minute)
			err := store.CloseCurrentExchangeRate(context.Background(), CloseCurrentExchangeRateParams{
				UserID:       user.ID,
				FromCurrency: "THB",
				ToCurrency:   "USD",
				ValidTo:      &closeTime,
			})
			require.NoError(t, err)
		}
	}

	first, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		Limit:        5,
		Offset:       0,
	})
	require.NoError(t, err)
	require.Len(t, first, 5)

	second, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		Limit:        5,
		Offset:       5,
	})
	require.NoError(t, err)
	require.Len(t, second, 5)

	require.NotEqual(t, first[0].ID, second[0].ID)
}

func TestListExchangeRates_UserIsolation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	otherUser := createTestUser(t, store)

	createTestExchangeRate(t, store, user.ID, "THB", "USD", "35", time.Now())

	result, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       otherUser.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		Limit:        10,
	})

	require.NoError(t, err)
	require.Empty(t, result)
}
func TestListExchangeRates_ZeroLimit(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	result, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       user.ID,
		FromCurrency: "THB",
		ToCurrency:   "USD",
		Limit:        0,
	})

	require.NoError(t, err)
	require.Empty(t, result)
}
