package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// RefreshRiskProfileTx atomically recalculates and persists a customer's risk
// profile. Locking the row first (FOR UPDATE) prevents two concurrent cron
// jobs from computing and writing stale scores simultaneously.

type RefreshRiskProfileTxParams struct {
	Profile     UpsertCustomerRiskProfileParams
	EventParams CreateCustomerEventParams // type should be CustomerEventTypeRiskScoreChanged
}

type RefreshRiskProfileTxResult struct {
	Profile CustomerRiskProfile
	Event   CustomerEvent
}

func (s *SQLStore) RefreshRiskProfileTx(ctx context.Context, arg RefreshRiskProfileTxParams) (RefreshRiskProfileTxResult, error) {
	var result RefreshRiskProfileTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		// 0. Verify the customer belongs to the caller before touching any risk data.
		if _, err := q.GetCustomerByID(ctx, GetCustomerByIDParams{
			ID:     arg.Profile.CustomerID,
			UserID: arg.Profile.UserID,
		}); err != nil {
			return err
		}

		// 1. Lock the row so concurrent cron ticks don't race.
		//    pgx.ErrNoRows is fine — first-run upsert will create the row.
		_, err := q.GetCustomerRiskProfileForUpdate(ctx, GetCustomerRiskProfileForUpdateParams{
			UserID:     arg.Profile.UserID,
			CustomerID: arg.Profile.CustomerID,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// 2. Write the new risk score
		result.Profile, err = q.UpsertCustomerRiskProfile(ctx, arg.Profile)
		if err != nil {
			return err
		}

		// 3. Touch the recalculation timestamp
		if tErr := q.TouchRiskProfileCalculated(ctx, TouchRiskProfileCalculatedParams{
			UserID:     arg.Profile.UserID,
			CustomerID: arg.Profile.CustomerID,
		}); tErr != nil {
			return tErr
		}

		// 4. Emit audit event so service layer and analytics can observe the change
		result.Event, err = q.CreateCustomerEvent(ctx, arg.EventParams)
		return err
	})

	return result, err
}
