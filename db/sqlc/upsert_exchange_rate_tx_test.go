package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertExchangeRateTx_Success(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	res, err := testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "35.0",
		Source:       string(RateSourceManual),
	})
	require.NoError(t, err)

	require.Equal(t, "USD", res.Forward.FromCurrency)
	require.Equal(t, "THB", res.Forward.ToCurrency)

	require.Equal(t, "THB", res.Inverse.FromCurrency)
	require.Equal(t, "USD", res.Inverse.ToCurrency)
}

func TestUpsertExchangeRateTx_InverseCorrectness(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	res, err := testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "EUR",
		Rate:         "2.0",
		Source:       string(RateSourceManual),
	})
	require.NoError(t, err)

	require.Equal(t, "0.5000000000", res.Inverse.Rate)
}

func TestUpsertExchangeRateTx_Replace(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	_, _ = testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "35",
		Source:       string(RateSourceManual),
	})

	res, err := testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "36",
		Source:       string(RateSourceManual),
	})
	require.NoError(t, err)

	require.Equal(t, "36.0000000000", res.Forward.Rate)
}

func TestUpsertExchangeRateTx_InvalidRate(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	_, err := testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "abc",
		Source:       string(RateSourceManual),
	})

	require.Error(t, err)
}

func TestUpsertExchangeRateTx_ZeroRate(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	_, err := testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "0",
		Source:       string(RateSourceManual),
	})

	require.Error(t, err)
}

func TestUpsertExchangeRateTx_NegativeRate(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	_, err := store.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "-10",
		Source:       string(RateSourceManual),
	})

	require.Error(t, err)
}

func TestUpsertExchangeRateTx_RollbackOnInverseFail(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	// simulate failure by invalid currency (inverse step fails)
	_, err := store.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "", // invalid → inverse insert fails
		Rate:         "35",
		Source:       string(RateSourceManual),
	})

	require.Error(t, err)

	// ensure nothing persisted
	rates, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID: user.ID,
		Limit:  10,
	})
	require.NoError(t, err)

	require.Empty(t, rates)
}

func TestUpsertExchangeRateTx_HighPrecision(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	res, err := testStore.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "BTC",
		Rate:         "0.00000001",
		Source:       string(RateSourceManual),
	})
	require.NoError(t, err)

	require.NotEmpty(t, res.Inverse.Rate)
}

func TestUpsertExchangeRateTx_UserIsolation(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)
	other := createTestUser(t, store)

	_, _ = store.UpsertExchangeRateTx(context.Background(), UpsertExchangeRateTxParams{
		UserID:       user.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Rate:         "35",
		Source:       string(RateSourceManual),
	})

	rates, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID: other.ID,
		Limit:  10,
	})
	require.NoError(t, err)

	require.Empty(t, rates)
}
