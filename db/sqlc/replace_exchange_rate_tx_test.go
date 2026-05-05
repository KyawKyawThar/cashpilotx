package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createRateTx(
	t *testing.T,
	store *SQLStore,
	userID uuid.UUID,
	from, to, rate string,
) ExchangeRate {

	arg := CreateExchangeRateParams{
		UserID:       userID,
		FromCurrency: from,
		ToCurrency:   to,
		Rate:         rate,
		Source:       RateSourceManual,
	}

	r, err := store.ReplaceExchangeRateTx(context.Background(), arg)
	require.NoError(t, err)

	return r
}
func TestReplaceExchangeRateTx(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)
	cleanupUser(t, user.ID)

	r1 := createRateTx(t, store, user.ID, "USD", "THB", "35.50")
	require.Nil(t, r1.ValidTo)

	time.Sleep(time.Millisecond * 5)

	r2 := createRateTx(t, store, user.ID, "USD", "THB", "36.00")

	// old should be closed
	old, err := store.GetExchangeRateAtTime(context.Background(), GetExchangeRateAtTimeParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		ValidFrom:    r1.ValidFrom.Add(time.Millisecond),
	})
	require.NoError(t, err)
	require.NotNil(t, old.ValidTo)
	require.False(t, old.ValidTo.After(time.Now()))

	// new active
	active, err := store.GetActiveExchangeRate(context.Background(), GetActiveExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
	})
	require.NoError(t, err)
	require.Equal(t, r2.ID, active.ID)
}
func TestExchangeRate_ConcurrentReplace(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)
	cleanupUser(t, user.ID)
	n := 10
	errs := make(chan error, n)

	for i := range n {
		go func(i int) {
			_, err := store.ReplaceExchangeRateTx(context.Background(), CreateExchangeRateParams{
				UserID:       user.ID,
				FromCurrency: "USD",
				ToCurrency:   "THB",
				Rate:         fmt.Sprintf("%d", 30+i),
				Source:       RateSourceManual,
			})
			errs <- err
		}(i)
	}

	for i := 0; i < n; i++ {
		require.NoError(t, <-errs)
	}

	// only one active must exist
	list, _ := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Limit:        100,
		Offset:       0,
	})

	activeCount := 0
	for _, r := range list {
		if r.ValidTo == nil {
			activeCount++
		}
	}

	require.Equal(t, 1, activeCount)
}
func TestExchangeRate_TimeBoundary(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)
	cleanupUser(t, user.ID)

	createRateTx(t, store, user.ID, "USD", "MMK", "2000")
	time.Sleep(time.Millisecond * 5)
	r2 := createRateTx(t, store, user.ID, "USD", "MMK", "2100")

	// exact boundary
	got, err := store.GetExchangeRateAtTime(context.Background(), GetExchangeRateAtTimeParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "MMK",
		ValidFrom:    r2.ValidFrom,
	})
	require.NoError(t, err)
	require.Equal(t, r2.ID, got.ID)
}
func TestExchangeRate_NoOverlapConstraint(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)

	now := time.Now()

	// first insert
	_, err := store.CreateExchangeRate(context.Background(), CreateExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "EUR",
		Rate:         "1.10",
		Source:       RateSourceManual,
		ValidFrom:    now,
	})
	require.NoError(t, err)

	// overlapping insert (should fail)
	_, err = store.CreateExchangeRate(context.Background(), CreateExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "EUR",
		Rate:         "1.20",
		Source:       RateSourceManual,
		ValidFrom:    now.Add(time.Second * -1),
	})
	require.Error(t, err)
}
func TestExchangeRate_InvalidRate(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)

	_, err := store.CreateExchangeRate(context.Background(), CreateExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "0",
		Source:       RateSourceManual,
		ValidFrom:    time.Now(),
	})
	require.Error(t, err)
}
func TestExchangeRate_InvalidCurrency(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)
	cleanupUser(t, user.ID)
	_, err := store.CreateExchangeRate(context.Background(), CreateExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "usd", // lowercase
		ToCurrency:   "THB",
		Rate:         "35.5",
		Source:       RateSourceManual,
		ValidFrom:    time.Now(),
	})
	require.Error(t, err)
}
func TestExchangeRate_SameCurrency(t *testing.T) {
	store := NewStore(testDB)
	user := createTestUser(t, store)
	cleanupUser(t, user.ID)
	cleanupUser(t, user.ID)
	_, err := store.CreateExchangeRate(context.Background(), CreateExchangeRateParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "USD",
		Rate:         "1.0",
		Source:       RateSourceManual,
		ValidFrom:    time.Now(),
	})
	require.Error(t, err)
}

func TestExchangeRate_UserIsolation(t *testing.T) {
	store := NewStore(testDB)

	user1 := createTestUser(t, store)
	user2 := createTestUser(t, store)
	cleanupUser(t, user1.ID)
	cleanupUser(t, user2.ID)
	createRateTx(t, store, user1.ID, "USD", "THB", "35")
	createRateTx(t, store, user2.ID, "USD", "THB", "40")

	r1, _ := store.GetActiveExchangeRate(context.Background(), GetActiveExchangeRateParams{
		UserID:       user1.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
	})

	r2, _ := store.GetActiveExchangeRate(context.Background(), GetActiveExchangeRateParams{
		UserID:       user2.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
	})

	require.NotEqual(t, r1.Rate, r2.Rate)
}
