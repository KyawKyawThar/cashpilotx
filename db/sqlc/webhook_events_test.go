package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestWebhookEvent(t *testing.T, store *SQLStore, userID *uuid.UUID) CreateWebhookEventRow {
	t.Helper()

	payload := json.RawMessage(`{"ok": true}`)

	ev, err := store.CreateWebhookEvent(context.Background(), CreateWebhookEventParams{
		Provider:  "stripe",
		EventID:   uuid.New().String(),
		UserID:    userID,
		Payload:   payload,
		Headers:   nil,
		Signature: nil,
	})

	require.NoError(t, err)
	require.Equal(t, "pending", ev.Status)

	return ev
}
func TestCreateWebhookEvent_Idempotent(t *testing.T) {
	store := newTestStore(t)

	user := createTestUser(t, store)

	ev1 := createTestWebhookEvent(t, store, &user.ID)

	// duplicate insert
	ev2, err := store.CreateWebhookEvent(context.Background(), CreateWebhookEventParams{
		Provider: ev1.Provider,
		EventID:  ev1.EventID,
		UserID:   &user.ID,
		Payload:  ev1.Payload,
	})

	require.NoError(t, err)

	// 🔥 SAME ROW (idempotency)
	require.Equal(t, ev1.ID, ev2.ID)
}
func TestCreateWebhookEvent_InvalidProvider(t *testing.T) {
	store := newTestStore(t)

	_, err := store.CreateWebhookEvent(context.Background(), CreateWebhookEventParams{
		Provider: "invalid", // violates CHECK
		EventID:  uuid.New().String(),
		Payload:  []byte(`{}`),
	})

	require.Error(t, err)
}
func TestPickWebhookEvents_Success(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	list, err := store.PickWebhookEvents(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, list, 1)

	require.Equal(t, ev.ID, list[0].ID)
	require.Equal(t, "processing", list[0].Status)
}
func TestPickWebhookEvents_NotReady(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	// set retry in future
	future := time.Now().Add(time.Hour)

	_, err := store.db.Exec(context.Background(), `
		UPDATE webhook_events
		SET next_retry_at = $1
		WHERE id = $2
	`, future, ev.ID)
	require.NoError(t, err)

	list, err := store.PickWebhookEvents(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, list, 0)
}
func TestMarkWebhookProcessed(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	list, _ := store.PickWebhookEvents(context.Background(), 1)
	require.Len(t, list, 1)

	err := store.MarkWebhookProcessed(context.Background(), list[0].ID)
	require.NoError(t, err)

	final, err := store.GetWebhookEvent(context.Background(), GetWebhookEventParams{
		Provider: ev.Provider,
		EventID:  ev.EventID,
	})
	require.NoError(t, err)

	require.Equal(t, "processed", final.Status)
	require.NotNil(t, final.ProcessedAt)
}
func TestMarkWebhookProcessed_Fail_NotProcessing(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	err := store.MarkWebhookProcessed(context.Background(), ev.ID)

	// no row updated → still success at SQL level, but state unchanged
	require.NoError(t, err)

	final, _ := store.GetWebhookEvent(context.Background(), GetWebhookEventParams{
		Provider: ev.Provider,
		EventID:  ev.EventID,
	})

	require.Equal(t, "pending", final.Status)
}
func TestMarkWebhookFailed_Retry(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	list, _ := store.PickWebhookEvents(context.Background(), 1)
	require.Len(t, list, 1)

	errMsg := "temporary failure"
	next := time.Now().Add(-time.Minute)

	err := store.MarkWebhookFailed(context.Background(), MarkWebhookFailedParams{
		ID:          list[0].ID,
		LastError:   &errMsg,
		NextRetryAt: &next,
	})
	require.NoError(t, err)

	final, _ := store.GetWebhookEvent(context.Background(), GetWebhookEventParams{
		Provider: ev.Provider,
		EventID:  ev.EventID,
	})

	require.Equal(t, "pending", final.Status)
	require.Equal(t, int32(1), final.AttemptCount)
}
func TestMarkWebhookDead(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	list, _ := store.PickWebhookEvents(context.Background(), 1)
	require.Len(t, list, 1)

	errMsg := "fatal"

	err := store.MarkWebhookDead(context.Background(), MarkWebhookDeadParams{
		ID:        list[0].ID,
		LastError: &errMsg,
	})
	require.NoError(t, err)

	final, _ := store.GetWebhookEvent(context.Background(), GetWebhookEventParams{
		Provider: ev.Provider,
		EventID:  ev.EventID,
	})

	require.Equal(t, "failed", final.Status)
}
func TestRequeueWebhookEvent(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	list, _ := store.PickWebhookEvents(context.Background(), 1)

	err := store.MarkWebhookProcessed(context.Background(), list[0].ID)
	require.NoError(t, err)

	err = store.RequeueWebhookEvent(context.Background(), ev.ID)
	require.NoError(t, err)

	final, _ := store.GetWebhookEvent(context.Background(), GetWebhookEventParams{
		Provider: ev.Provider,
		EventID:  ev.EventID,
	})

	require.Equal(t, "pending", final.Status)
}
func TestWebhookEvent_NullUser(t *testing.T) {
	store := newTestStore(t)

	ev := createTestWebhookEvent(t, store, nil)

	require.Nil(t, ev.UserID)
}
