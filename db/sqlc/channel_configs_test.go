package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func randomBytes() []byte {
	return []byte(uuid.New().String())
}

func createTestChannelConfig(t *testing.T, store *SQLStore, userID uuid.UUID, channel string) ChannelConfig {
	t.Helper()

	arg := UpsertChannelConfigParams{
		UserID:          userID,
		Channel:         channel,
		ConfigEncrypted: randomBytes(),
		ConfigNonce:     randomBytes(),
		ConfigTag:       randomBytes(),
		KeyID:           "test-key",
		ConfigVersion:   1,
		IsActive:        true,
		Status:          "connected",
	}

	config, err := store.UpsertChannelConfig(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, config.ID)

	return config
}
func TestUpsertChannelConfig_Create(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	config := createTestChannelConfig(t, store, user.ID, "messenger")

	require.Equal(t, user.ID, config.UserID)
	require.Equal(t, "messenger", config.Channel)
	require.Equal(t, int32(1), config.ConfigVersion)
	require.True(t, config.IsActive)
}
func TestGetActiveChannelConfig(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	created := createTestChannelConfig(t, store, user.ID, "line")

	got, err := store.GetActiveChannelConfig(context.Background(), GetActiveChannelConfigParams{
		UserID:  user.ID,
		Channel: "line",
	})

	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
}
func TestUpsertChannelConfig_ConflictVersionIncrement(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	first := createTestChannelConfig(t, store, user.ID, "telegram")

	second := createTestChannelConfig(t, store, user.ID, "telegram")

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, int32(2), second.ConfigVersion)
}
func TestChannelConfig_SoftDelete_AllowsRecreate(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	config := createTestChannelConfig(t, store, user.ID, "messenger")

	err := store.DeleteChannelConfig(context.Background(), config.ID)
	require.NoError(t, err)

	newConfig := createTestChannelConfig(t, store, user.ID, "messenger")

	require.NotEqual(t, config.ID, newConfig.ID)
}
func TestGetActiveChannelConfig_Inactive(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	config := createTestChannelConfig(t, store, user.ID, "line")

	err := store.SetChannelActive(context.Background(), SetChannelActiveParams{
		ID:       config.ID,
		IsActive: false,
	})
	require.NoError(t, err)

	_, err = store.GetActiveChannelConfig(context.Background(), GetActiveChannelConfigParams{
		UserID:  user.ID,
		Channel: "line",
	})

	require.Error(t, err)
}
func TestChannelConfig_InvalidChannel(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)

	_, err := store.UpsertChannelConfig(context.Background(), UpsertChannelConfigParams{
		UserID:          user.ID,
		Channel:         "invalid_channel",
		ConfigEncrypted: randomBytes(),
		ConfigNonce:     randomBytes(),
		ConfigTag:       randomBytes(),
		KeyID:           "key",
		ConfigVersion:   1,
		IsActive:        true,
		Status:          "connected",
	})

	require.Error(t, err)
}
func TestChannelConfig_InvalidStatus(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	_, err := store.UpsertChannelConfig(context.Background(), UpsertChannelConfigParams{
		UserID:          user.ID,
		Channel:         "line",
		ConfigEncrypted: randomBytes(),
		ConfigNonce:     randomBytes(),
		ConfigTag:       randomBytes(),
		KeyID:           "key",
		ConfigVersion:   1,
		IsActive:        true,
		Status:          "unknown",
	})

	require.Error(t, err)
}
func TestMarkChannelError(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	config := createTestChannelConfig(t, store, user.ID, "telegram")

	errMsg := "connection failed"

	err := store.MarkChannelError(context.Background(), MarkChannelErrorParams{
		ID:        config.ID,
		LastError: &errMsg,
	})
	require.NoError(t, err)

	updated, _ := store.GetActiveChannelConfig(context.Background(), GetActiveChannelConfigParams{
		UserID:  user.ID,
		Channel: "telegram",
	})

	require.Equal(t, "error", updated.Status)
	require.NotNil(t, updated.LastError)
}
func TestMarkChannelConnected(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	user := createTestUser(t, store)
	config := createTestChannelConfig(t, store, user.ID, "whatsapp")

	errMsg := "fail"
	_ = store.MarkChannelError(context.Background(), MarkChannelErrorParams{
		ID:        config.ID,
		LastError: &errMsg,
	})

	err := store.MarkChannelConnected(context.Background(), config.ID)
	require.NoError(t, err)

	updated, _ := store.GetActiveChannelConfig(context.Background(), GetActiveChannelConfigParams{
		UserID:  user.ID,
		Channel: "whatsapp",
	})

	require.Equal(t, "connected", updated.Status)
	require.Nil(t, updated.LastError)
	require.NotNil(t, updated.LastConnectedAt)
}
