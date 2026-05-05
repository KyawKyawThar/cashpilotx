package db

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
)

// UpsertExchangeRateTx atomically writes the forward rate (From→To) and its
// arithmetic inverse (To→From = 1/rate), keeping both directions consistent.

type UpsertExchangeRateTxParams struct {
	UserID       uuid.UUID
	FromCurrency string
	ToCurrency   string
	Rate         string // e.g. "35.00" for THB→MMK
	Source       string
}

type UpsertExchangeRateTxResult struct {
	Forward ExchangeRate
	Inverse ExchangeRate
}

func (s *SQLStore) UpsertExchangeRateTx(ctx context.Context, arg UpsertExchangeRateTxParams) (UpsertExchangeRateTxResult, error) {
	var result UpsertExchangeRateTxResult

	rateF, ok := new(big.Float).SetPrec(128).SetString(arg.Rate)
	if !ok || rateF.Sign() <= 0 {
		return result, fmt.Errorf("invalid exchange rate %q: must be a positive number", arg.Rate)
	}

	one := new(big.Float).SetPrec(128).SetFloat64(1)
	inverseStr := new(big.Float).SetPrec(128).Quo(one, rateF).Text('f', 10)

	err := s.execTx(ctx, func(q *Queries) error {
		now := time.Now()
		src := RateSource(arg.Source)

		replace := func(from, to, rate string) (ExchangeRate, error) {
			if err := q.CloseCurrentExchangeRate(ctx, CloseCurrentExchangeRateParams{
				UserID:       arg.UserID,
				FromCurrency: from,
				ToCurrency:   to,
				ValidTo:      &now,
			}); err != nil {
				return ExchangeRate{}, err
			}
			return q.CreateExchangeRate(ctx, CreateExchangeRateParams{
				UserID:       arg.UserID,
				FromCurrency: from,
				ToCurrency:   to,
				Rate:         rate,
				Source:       src,
				ValidFrom:    now,
			})
		}

		var err error
		result.Forward, err = replace(arg.FromCurrency, arg.ToCurrency, arg.Rate)
		if err != nil {
			return err
		}
		result.Inverse, err = replace(arg.ToCurrency, arg.FromCurrency, inverseStr)
		return err
	})

	return result, err
}
