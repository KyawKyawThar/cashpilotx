package db

import (
	"context"
	"time"
)

// RegisterUserTx atomically creates a user, their first session, and optional
// seed exchange rates. On any failure the whole thing rolls back so the user
// can retry registration cleanly.
// Enforces a max of 5 active sessions per user (oldest deleted first).

type RegisterUserTxParams struct {
	User         CreateUserParams
	Session      CreateSessionParams
	DefaultRates []CreateExchangeRateParams
}

type RegisterUserTxResult struct {
	User    User
	Session Session
}

func (s *SQLStore) RegisterUserTx(ctx context.Context, arg RegisterUserTxParams) (RegisterUserTxResult, error) {
	var result RegisterUserTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error

		result.User, err = q.CreateUser(ctx, arg.User)
		if err != nil {
			return err
		}

		arg.Session.UserID = result.User.ID
		result.Session, err = q.CreateSession(ctx, arg.Session)
		if err != nil {
			return err
		}

		// Enforce session cap (max 5 per user, delete oldest first)
		const maxSessions = 5
		sessions, err := q.ListUserSessions(ctx, result.User.ID)
		if err != nil {
			return err
		}
		// ListUserSessions returns oldest-first; trim the head if over cap
		for i := 0; i < len(sessions)-maxSessions; i++ {
			if _, err := q.DeleteSessionByID(ctx, sessions[i].ID); err != nil {
				return err
			}
		}

		// Seed default exchange rates with user ID propagated
		for i := range arg.DefaultRates {
			arg.DefaultRates[i].UserID = result.User.ID
			now := time.Now()
			if err := q.CloseCurrentExchangeRate(ctx, CloseCurrentExchangeRateParams{
				UserID:       arg.DefaultRates[i].UserID,
				FromCurrency: arg.DefaultRates[i].FromCurrency,
				ToCurrency:   arg.DefaultRates[i].ToCurrency,
				ValidTo:      &now,
			}); err != nil {
				return err
			}
			arg.DefaultRates[i].ValidFrom = now
			if _, err := q.CreateExchangeRate(ctx, arg.DefaultRates[i]); err != nil {
				return err
			}
		}

		return nil
	})

	return result, err
}
