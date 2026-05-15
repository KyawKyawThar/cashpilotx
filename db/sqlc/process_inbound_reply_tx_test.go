package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestProcessInboundReplyTx_Promised(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "whatsapp",
		Content:    "I'll pay tomorrow",
		Tone:       "polite",
	}

	res, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         user.ID,
		CustomerID:     customer.ID,
		Classification: ReplyClassificationPromised,
		Message:        msg,
	})

	require.NoError(t, err)

	require.Equal(t, "promised", res.Invoice.CollectionState)
	require.Equal(t, "inbound", res.Message.Direction)
	require.Equal(t, CustomerEventTypeReplyReceived, res.Event.EventType)
}
func TestProcessInboundReplyTx_Negotiating(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "messenger",
		Content:    "Can we discuss?",
		Tone:       "firm",
	}

	res, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         user.ID,
		CustomerID:     customer.ID,
		Classification: ReplyClassificationNegotiating,
		Message:        msg,
	})

	require.NoError(t, err)
	require.Equal(t, "negotiating", res.Invoice.CollectionState)
}
func TestProcessInboundReplyTx_PaidClaimMapsToVerifying(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "line",
		Content:    "I already paid",
		Tone:       "polite",
	}

	res, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         user.ID,
		CustomerID:     customer.ID,
		Classification: ReplyClassificationPaidClaim,
		Message:        msg,
	})

	require.NoError(t, err)
	require.Equal(t, "verifying", res.Invoice.CollectionState)
}
func TestProcessInboundReplyTx_WrongUser(t *testing.T) {
	user := committedUser(t)
	other := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     other.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "whatsapp",
		Content:    "hello",
		Tone:       "polite",
	}

	_, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         other.ID,
		CustomerID:     customer.ID,
		Classification: ReplyClassificationPromised,
		Message:        msg,
	})

	require.Error(t, err)
}
func TestProcessInboundReplyTx_InvalidMessage(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "whatsapp",
		Content:    "", // invalid
		Tone:       "neutral",
	}

	_, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         user.ID,
		CustomerID:     customer.ID,
		Classification: ReplyClassificationPromised,
		Message:        msg,
	})

	require.Error(t, err)
}

func TestProcessInboundReplyTx_RollbackOnEventFailure(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "whatsapp",
		Content:    "test",
		Tone:       "neutral",
	}

	_, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         user.ID,
		CustomerID:     uuid.New(), // invalid FK → event fails
		Classification: ReplyClassificationPromised,
		Message:        msg,
	})

	require.Error(t, err)

	// ensure state NOT updated
	check := getInvoice(t, inv.ID, user.ID)

	require.NotEqual(t, "promised", check.CollectionState)
}
func TestProcessInboundReplyTx_UnknownClassification(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "whatsapp",
		Content:    "random reply",
		Tone:       "polite",
	}

	res, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
		InvoiceID:      inv.ID,
		UserID:         user.ID,
		CustomerID:     customer.ID,
		Classification: ReplyClassificationOther,
		Message:        msg,
	})

	require.NoError(t, err)
	require.Equal(t, "negotiating", res.Invoice.CollectionState)
}
func TestProcessInboundReplyTx_MultipleReplies(t *testing.T) {
	user := committedUser(t)
	customer := committedCustomer(t, user.ID)
	inv := committedInvoice(t, user.ID, customer.ID)

	msg := CreateMessageParams{
		UserID:     user.ID,
		CustomerID: customer.ID,
		InvoiceID:  &inv.ID,
		Direction:  "inbound",
		Channel:    "whatsapp",
		Content:    "first",
		Tone:       "polite",
	}

	for range 3 {
		_, err := testStore.ProcessInboundReplyTx(context.Background(), ProcessInboundReplyTxParams{
			InvoiceID:      inv.ID,
			UserID:         user.ID,
			CustomerID:     customer.ID,
			Classification: ReplyClassificationNegotiating,
			Message:        msg,
		})
		require.NoError(t, err)
	}

	final := getInvoice(t, inv.ID, user.ID)
	require.Equal(t, "negotiating", final.CollectionState)
}
