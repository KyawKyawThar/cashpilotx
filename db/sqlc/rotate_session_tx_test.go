package db

import (
	"cashpilotx/utils"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRotateSessionTx_Success(t *testing.T) {
	ua := "test"
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	arg := RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(32),
			UserAgent:    &ua,
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	}

	res, err := testStore.RotateSessionTx(context.Background(), arg)
	require.NoError(t, err)

	require.NotZero(t, res.Session.ID)
	require.Equal(t, user.ID, res.Session.UserID)
}

func TestRotateSessionTx_ExactlyFive(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	for i := 0; i < 5; i++ {
		_, err := testStore.CreateSession(context.Background(), CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(16),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		})
		require.NoError(t, err)
	}

	_, err := testStore.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(32),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	sessions, err := testStore.ListUserSessions(context.Background(), user.ID)
	require.NoError(t, err)

	require.Len(t, sessions, 5)
}

func TestRotateSessionTx_DeleteOldest(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	// create 5 sessions
	var firstID = ""
	for i := 0; i < 5; i++ {
		s, err := testStore.CreateSession(context.Background(), CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(16),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		})
		require.NoError(t, err)

		if i == 0 {
			firstID = s.ID.String()
		}
	}

	// rotate → should evict oldest
	_, err := testStore.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(32),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	sessions, err := testStore.ListUserSessions(context.Background(), user.ID)
	require.NoError(t, err)

	require.Len(t, sessions, 5)

	// ensure oldest removed
	for _, s := range sessions {
		require.NotEqual(t, firstID, s.ID.String())
	}
}
func TestRotateSessionTx_ManyOverflow(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	// create 10 sessions
	for i := 0; i < 10; i++ {
		_, err := testStore.CreateSession(context.Background(), CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(16),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		})
		require.NoError(t, err)
	}

	_, err := testStore.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(32),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	sessions, err := testStore.ListUserSessions(context.Background(), user.ID)
	require.NoError(t, err)

	require.Len(t, sessions, 5)
}

func TestRotateSessionTx_InvalidToken(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	_, err := store.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: "", // invalid
		},
	})

	require.Error(t, err)
}

func TestRotateSessionTx_InvalidUser(t *testing.T) {
	store := newTestStore(t)

	_, err := store.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       utils.RandomUUID(),
			RefreshToken: utils.RandomString(32),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	})

	require.Error(t, err)
}
func TestRotateSessionTx_RollbackOnCreateFail(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	// create baseline sessions
	for i := 0; i < 3; i++ {
		_, _ = testStore.CreateSession(context.Background(), CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(32),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		})
	}

	_, err := testStore.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: "", // force failure
		},
	})

	require.Error(t, err)

	sessions, err := testStore.ListUserSessions(context.Background(), user.ID)
	require.NoError(t, err)

	// should remain unchanged
	require.Len(t, sessions, 3)
}
func TestRotateSessionTx_UserIsolation(t *testing.T) {
	user := createTestUser(t, testStore.(*SQLStore))
	other := createTestUser(t, testStore.(*SQLStore))
	t.Cleanup(func() {
		_, _ = testDB.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	// sessions for user
	for i := 0; i < 5; i++ {
		_, _ = testStore.CreateSession(context.Background(), CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(16),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		})
	}

	// sessions for other user
	for i := 0; i < 3; i++ {
		_, _ = testStore.CreateSession(context.Background(), CreateSessionParams{
			UserID:       other.ID,
			RefreshToken: utils.RandomString(16),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		})
	}

	_, err := testStore.RotateSessionTx(context.Background(), RotateSessionTxParams{
		Session: CreateSessionParams{
			UserID:       user.ID,
			RefreshToken: utils.RandomString(32),
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	})
	require.NoError(t, err)

	userSessions, _ := testStore.ListUserSessions(context.Background(), user.ID)
	otherSessions, _ := testStore.ListUserSessions(context.Background(), other.ID)

	require.Len(t, userSessions, 5)
	require.Len(t, otherSessions, 3)
}
