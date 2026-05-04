package db

import (
	"cashpilotx/utils"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// -----------------------------
// Helpers
// -----------------------------

func newRegisterParams() RegisterUserTxParams {
	ua := "test"
	return RegisterUserTxParams{
		User: CreateUserParams{
			Phone:        utils.RandomPhone(9),
			PinHash:      "hash",
			Name:         utils.RandomString(6),
			Plan:         "pro",
			HomeCurrency: "USD",
		},
		Session: CreateSessionParams{
			RefreshToken: utils.RandomString(32),
			UserAgent:    &ua,
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	}
}

// -----------------------------
// SUCCESS (NO RATES)
// -----------------------------

func TestRegisterUserTx_Success_NoRates(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()

	res, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)

	require.NotZero(t, res.User.ID)
	require.Equal(t, res.User.ID, res.Session.UserID)
}

// -----------------------------
// SUCCESS WITH RATES
// -----------------------------

func TestRegisterUserTx_WithRates(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()
	arg.DefaultRates = []CreateExchangeRateParams{
		{
			FromCurrency: "USD",
			ToCurrency:   "THB",
			Rate:         "35.0",
			Source:       RateSourceManual,
			// UserID + ValidFrom set inside Tx
		},
	}

	res, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)

	rates, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       res.User.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Limit:        10,
	})
	require.NoError(t, err)
	require.Len(t, rates, 1)
}

// -----------------------------
// MULTIPLE RATES
// -----------------------------

func TestRegisterUserTx_MultipleRates(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()
	arg.DefaultRates = []CreateExchangeRateParams{
		{
			FromCurrency: "USD",
			ToCurrency:   "THB",
			Rate:         "35",
			Source:       RateSourceManual,
		},
		{
			FromCurrency: "USD",
			ToCurrency:   "MMK",
			Rate:         "2100",
			Source:       RateSourceManual,
		},
	}

	res, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)

	thb, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       res.User.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Limit:        10,
	})
	require.NoError(t, err)
	require.Len(t, thb, 1)

	mmk, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       res.User.ID,
		FromCurrency: "USD",
		ToCurrency:   "MMK",
		Limit:        10,
	})
	require.NoError(t, err)
	require.Len(t, mmk, 1)
}

// -----------------------------
// ATOMICITY: SESSION FAIL
// -----------------------------

func TestRegisterUserTx_RollbackOnSessionFail(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()

	// invalid session
	arg.Session.RefreshToken = ""

	_, err := store.RegisterUserTx(context.Background(), arg)
	require.Error(t, err)

	// ensure user not persisted
	_, err = store.GetUserByPhone(context.Background(), arg.User.Phone)
	require.Error(t, err)
}

// -----------------------------
// ATOMICITY: RATE FAIL
// -----------------------------

func TestRegisterUserTx_RollbackOnRateFail(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()
	arg.DefaultRates = []CreateExchangeRateParams{
		{
			FromCurrency: "USD",
			ToCurrency:   "", // invalid
			Rate:         "35",
			Source:       RateSourceManual,
		},
	}

	_, err := store.RegisterUserTx(context.Background(), arg)
	require.Error(t, err)

	// user should not exist
	_, err = store.GetUserByPhone(context.Background(), arg.User.Phone)
	require.Error(t, err)
}

// -----------------------------
// DUPLICATE USER
// -----------------------------

func TestRegisterUserTx_DuplicateUser(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()

	_, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)

	_, err = store.RegisterUserTx(context.Background(), arg)
	require.Error(t, err)
}

// -----------------------------
// EDGE: EMPTY RATES
// -----------------------------

func TestRegisterUserTx_EmptyRates(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()
	arg.DefaultRates = []CreateExchangeRateParams{}

	res, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)
	require.NotZero(t, res.User.ID)
}

// -----------------------------
// EDGE: RATE VALIDFROM OVERRIDE
// -----------------------------

func TestRegisterUserTx_ValidFromOverride(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()
	arg.DefaultRates = []CreateExchangeRateParams{
		{
			FromCurrency: "USD",
			ToCurrency:   "THB",
			Rate:         "35",
			Source:       RateSourceManual,
			ValidFrom:    time.Now().Add(-24 * time.Hour), // should be overridden
		},
	}

	res, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)

	rates, err := store.ListExchangeRates(context.Background(), ListExchangeRatesParams{
		UserID:       res.User.ID,
		FromCurrency: "USD",
		ToCurrency:   "THB",
		Limit:        1,
	})
	require.NoError(t, err)

	// ValidFrom should be recent (set inside Tx)
	require.True(t, time.Since(rates[0].ValidFrom) < time.Minute)
}
