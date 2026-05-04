package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Helper
func createTestSession(t *testing.T, store *SQLStore, userID uuid.UUID, expiresAt time.Time) Session {
	t.Helper()

	if store == nil {
		store = newTestStore(t)
	}

	s, err := store.CreateSession(context.Background(), CreateSessionParams{
		UserID:       userID,
		RefreshToken: uuid.New().String(),
		UserAgent:    nil,
		IpAddress:    nil,
		DeviceID:     nil,
		ExpiresAt:    expiresAt,
	})
	require.NoError(t, err)
	require.NotZero(t, s.ID)

	return s
}

func TestCreateSession_Success(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	ua := "Chrome"
	ip := "127.0.0.1"
	device := "iphone"

	s, err := store.CreateSession(context.Background(), CreateSessionParams{
		UserID:       user.ID,
		RefreshToken: uuid.New().String(),
		UserAgent:    &ua,
		IpAddress:    &ip,
		DeviceID:     &device,
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	require.NoError(t, err)
	require.Equal(t, user.ID, s.UserID)
	require.Equal(t, ua, *s.UserAgent)
	require.Equal(t, ip, *s.IpAddress)
	require.Equal(t, device, *s.DeviceID)
}

func TestCreateSession_InvalidUser_Fails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.CreateSession(context.Background(), CreateSessionParams{
		UserID:       uuid.New(),
		RefreshToken: uuid.New().String(),
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	require.Error(t, err)
}
func TestGetSessionByRefreshToken_Expired_Fails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	s := createTestSession(t, store, user.ID, time.Now().Add(-time.Hour))

	_, err := store.GetSessionByRefreshToken(context.Background(), s.RefreshToken)
	require.Error(t, err)
}
func TestGetSessionByRefreshToken_NotFound(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.GetSessionByRefreshToken(context.Background(), uuid.New().String())
	require.Error(t, err)
}
func TestDeleteSessionByID_Success(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	s := createTestSession(t, store, user.ID, time.Now().Add(time.Hour))

	id, err := store.DeleteSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	require.Equal(t, s.ID, id)

	_, err = store.GetSessionByRefreshToken(context.Background(), s.RefreshToken)
	require.Error(t, err)
}
func TestDeleteSessionByID_NotFound_Fails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.DeleteSessionByID(context.Background(), uuid.New())
	require.Error(t, err)
}
func TestDeleteSessionByRefreshToken_NotFound_Fails(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.DeleteSessionByRefreshToken(context.Background(), uuid.New().String())
	require.Error(t, err)
}
func TestDeleteAllUserSessions_Isolated(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	u1 := createTestUser(t, store)
	u2 := createTestUser(t, store)

	createTestSession(t, store, u1.ID, time.Now().Add(time.Hour))
	active := createTestSession(t, store, u2.ID, time.Now().Add(time.Hour))

	err := store.DeleteAllUserSessions(context.Background(), u1.ID)
	require.NoError(t, err)

	// u2 session must still exist
	_, err = store.GetSessionByRefreshToken(context.Background(), active.RefreshToken)
	require.NoError(t, err)
}
func TestDeleteExpiredSessions_Boundary(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	expired := createTestSession(t, store, user.ID, time.Now().Add(-time.Second))
	active := createTestSession(t, store, user.ID, time.Now().Add(time.Hour))

	err := store.DeleteExpiredSessions(context.Background())
	require.NoError(t, err)

	// expired should be gone
	_, err = store.GetSessionByRefreshToken(context.Background(), expired.RefreshToken)
	require.Error(t, err)

	// active should remain
	_, err = store.GetSessionByRefreshToken(context.Background(), active.RefreshToken)
	require.NoError(t, err)
}
func TestListUserSessions_OrderAndFilter(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	old := createTestSession(t, store, user.ID, time.Now().Add(time.Hour))
	time.Sleep(10 * time.Millisecond)
	newer := createTestSession(t, store, user.ID, time.Now().Add(time.Hour))

	sessions, err := store.ListUserSessions(context.Background(), user.ID)
	require.NoError(t, err)

	require.Len(t, sessions, 2)

	// oldest first
	require.Equal(t, old.ID, sessions[0].ID)
	require.Equal(t, newer.ID, sessions[1].ID)
}
func TestListUserSessions_ExcludeExpired(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)

	createTestSession(t, store, user.ID, time.Now().Add(-time.Hour)) // expired
	createTestSession(t, store, user.ID, time.Now().Add(time.Hour))  // active

	sessions, err := store.ListUserSessions(context.Background(), user.ID)
	require.NoError(t, err)

	require.Len(t, sessions, 1)
}
