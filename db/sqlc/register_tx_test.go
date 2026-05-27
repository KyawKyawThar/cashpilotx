package db

import (
	"cashpilotx/utils"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newRegisterParams() RegisterUserTxParams {
	ua := "test"
	phone := utils.RandomPhone(9)
	bn := utils.RandomString(6)
	return RegisterUserTxParams{
		User: CreateUserParams{
			Phone:        &phone,
			PinHash:      "hash",
			Name:         utils.RandomString(6),
			Plan:         "pro",
			HomeCurrency: "THB",
			BusinessName: &bn,
		},
		Session: CreateSessionParams{
			RefreshToken: utils.RandomString(32),
			UserAgent:    &ua,
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	}
}

func TestRegisterUserTx_Success(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()

	res, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)
	require.NotZero(t, res.User.ID)
	require.Equal(t, res.User.ID, res.Session.UserID)
}

func TestRegisterUserTx_RollbackOnSessionFail(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()
	arg.Session.RefreshToken = ""

	_, err := store.RegisterUserTx(context.Background(), arg)
	require.Error(t, err)

	_, err = store.GetUserByPhone(context.Background(), arg.User.Phone)
	require.Error(t, err)
}

func TestRegisterUserTx_DuplicateUser(t *testing.T) {
	store := newTestStore(t)

	arg := newRegisterParams()

	_, err := store.RegisterUserTx(context.Background(), arg)
	require.NoError(t, err)

	_, err = store.RegisterUserTx(context.Background(), arg)
	require.Error(t, err)
}
