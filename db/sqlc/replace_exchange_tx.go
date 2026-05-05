package db

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
)

// exchangeRateLockKey maps (userID, fromCurrency, toCurrency) to a stable int64
// suitable for pg_advisory_xact_lock.
func exchangeRateLockKey(userID uuid.UUID, from, to string) int64 {
	h := fnv.New64a()
	h.Write(userID[:])
	h.Write([]byte(from))
	h.Write([]byte(to))
	return int64(h.Sum64()) //nolint:gosec // intentional wrap for pg_advisory_xact_lock key
}

func (store *SQLStore) ReplaceExchangeRateTx(
	ctx context.Context,
	arg CreateExchangeRateParams,
) (ExchangeRate, error) {

	var result ExchangeRate

	err := store.execTx(ctx, func(q *Queries) error {
		// Serialize concurrent replaces for the same currency pair so the
		// close→insert sequence is atomic even when no row exists yet.
		lockKey := exchangeRateLockKey(arg.UserID, arg.FromCurrency, arg.ToCurrency)
		if _, err := q.db.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
			return err
		}

		now := time.Now()

		// 1. close existing
		if err := q.CloseCurrentExchangeRate(ctx, CloseCurrentExchangeRateParams{
			UserID:       arg.UserID,
			FromCurrency: arg.FromCurrency,
			ToCurrency:   arg.ToCurrency,
			ValidTo:      &now,
		}); err != nil {
			return err
		}

		// 2. insert new
		var err error
		arg.ValidFrom = now
		result, err = q.CreateExchangeRate(ctx, arg)
		return err
	})

	return result, err
}
