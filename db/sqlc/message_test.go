package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestMessage(
	t *testing.T,
	store *SQLStore,
	userID uuid.UUID,
	customerID uuid.UUID,
	invoiceID *uuid.UUID,
) Message {
	t.Helper()

	meta := json.RawMessage(`{"k":"v"}`)

	msg, err := store.CreateMessage(context.Background(), CreateMessageParams{
		UserID:     userID,
		InvoiceID:  invoiceID,
		CustomerID: customerID,
		Direction:  "outbound",
		Channel:    "messenger",
		Content:    "hello",
		Tone:       "polite",
		Column8:    "", // default → draft
		Column9:    "", // default → ai
		Column10:   meta,
	})

	require.NoError(t, err)
	require.Equal(t, "draft", msg.Status)

	return msg
}
func TestCreateMessage_Success_Minimal(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	require.NotEmpty(t, msg.ID)
	require.Nil(t, msg.InvoiceID)
}
func TestCreateMessage_Success_WithInvoice(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)
	invoice := createTestInvoice(t, store, user.ID, customer.ID, 100)

	msg := createTestMessage(t, store, user.ID, customer.ID, &invoice.ID)

	require.NotNil(t, msg.InvoiceID)
	require.Equal(t, invoice.ID, *msg.InvoiceID)
}
func TestCreateMessage_Fail_InvalidFK(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.CreateMessage(context.Background(), CreateMessageParams{
		UserID:     uuid.New(),
		CustomerID: uuid.New(),
		Direction:  "outbound",
		Channel:    "messenger",
		Content:    "x",
		Tone:       "friendly",
	})

	require.Error(t, err)
}
func TestCreateMessage_LargeMeta(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	large := make(map[string]interface{})
	for i := range 1000 {
		large[uuid.NewString()] = i
	}

	meta, _ := json.Marshal(large)

	_, err := store.CreateMessage(context.Background(), CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		Direction:  "outbound",
		Channel:    "messenger",
		Content:    "hello",
		Tone:       "polite",
		Column10:   meta,
	})

	require.NoError(t, err)
}
func TestGetMessageByID_Success(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	res, err := store.GetMessageByID(context.Background(), msg.ID)

	require.NoError(t, err)
	require.Equal(t, msg.ID, res.ID)
}
func TestGetMessageByID_NotFound(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.GetMessageByID(context.Background(), uuid.New())
	require.Error(t, err)
}
func TestGetMessageByExternalID(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	ext := "ext-123"

	_, _ = store.MarkMessageSent(context.Background(), MarkMessageSentParams{
		ID: msg.ID, ExternalMessageID: &ext,
	})

	res, err := store.GetMessageByExternalID(context.Background(), GetMessageByExternalIDParams{
		Channel:           "messenger",
		ExternalMessageID: &ext,
	})

	require.NoError(t, err)
	require.Equal(t, msg.ID, res.ID)
}
func TestApproveMessage(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	res, err := store.ApproveMessage(context.Background(), msg.ID)

	require.NoError(t, err)
	require.Equal(t, "pending", res.Status)
}

func TestApproveMessage_Fail_NotDraft(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	_, _ = store.ApproveMessage(context.Background(), msg.ID)

	_, err := store.ApproveMessage(context.Background(), msg.ID)

	require.Error(t, err)
}
func TestDeliveryLifecycle(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	ext := "ext-xyz"

	sent, err := store.MarkMessageSent(context.Background(), MarkMessageSentParams{
		ID: msg.ID, ExternalMessageID: &ext,
	})
	require.NoError(t, err)
	require.Equal(t, "sent", sent.Status)

	delivered, err := store.MarkMessageDelivered(context.Background(), MarkMessageDeliveredParams{
		Channel:           "messenger",
		ExternalMessageID: &ext,
	})
	require.NoError(t, err)
	require.Equal(t, "delivered", delivered.Status)

	read, err := store.MarkMessageRead(context.Background(), MarkMessageReadParams{
		Channel:           "messenger",
		ExternalMessageID: &ext,
	})
	require.NoError(t, err)
	require.NotNil(t, read.ReadAt)
}
func TestMarkMessageDelivered_Idempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	ext := "ext"

	_, _ = store.MarkMessageSent(context.Background(), MarkMessageSentParams{
		ID: msg.ID, ExternalMessageID: &ext,
	})

	_, _ = store.MarkMessageDelivered(context.Background(), MarkMessageDeliveredParams{
		Channel: "messenger", ExternalMessageID: &ext,
	})

	_, err := store.MarkMessageDelivered(context.Background(), MarkMessageDeliveredParams{
		Channel: "messenger", ExternalMessageID: &ext,
	})

	require.NoError(t, err)
}
func TestFetchApprovedMessages(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)
	_, _ = store.ApproveMessage(context.Background(), msg.ID)

	list, err := store.FetchApprovedMessages(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, list, 1)
}
func TestDeleteMessage(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	user := createTestUser(t, store)
	customer := createTestCustomer(t, store, user.ID)

	msg := createTestMessage(t, store, user.ID, customer.ID, nil)

	err := store.DeleteMessage(context.Background(), msg.ID)
	require.NoError(t, err)

	_, err = store.GetMessageByID(context.Background(), msg.ID)
	require.Error(t, err)
}
