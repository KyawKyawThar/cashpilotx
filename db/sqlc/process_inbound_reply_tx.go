package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ReplyClassification is the caller-supplied intent classification of an
// inbound message. The AI or keyword classifier resolves this before calling
// ProcessInboundReplyTx.
type ReplyClassification string

const (
	ReplyClassificationPromised    ReplyClassification = "promised"    // "I'll pay tomorrow"
	ReplyClassificationPaidClaim   ReplyClassification = "paid_claim"  // "I already paid" (unconfirmed)
	ReplyClassificationNegotiating ReplyClassification = "negotiating" // "Can we discuss?"
	ReplyClassificationOther       ReplyClassification = "other"       // any other response
)

// collectionStateForReply maps a reply classification to the appropriate
// collection lifecycle state. paid_claim → 'promised' because the financial
// status only flips when RecordPaymentTx is called by the owner.
func collectionStateForReply(c ReplyClassification) string {
	switch c {
	case ReplyClassificationPromised, ReplyClassificationPaidClaim:
		return "promised"
	case ReplyClassificationNegotiating, ReplyClassificationOther:
		return "negotiating"
	default:
		return "negotiating"
	}
}

// ProcessInboundReplyTx atomically records an inbound customer message,
// advances the invoice collection_state based on the reply classification,
// and emits a reply_received event for the History timeline.

type ProcessInboundReplyTxParams struct {
	InvoiceID      uuid.UUID
	UserID         uuid.UUID
	CustomerID     uuid.UUID
	Classification ReplyClassification
	Message        CreateMessageParams // must be direction='inbound'
}

type ProcessInboundReplyTxResult struct {
	Invoice Invoice
	Message Message
	Event   CustomerEvent
}

func (s *SQLStore) ProcessInboundReplyTx(ctx context.Context, arg ProcessInboundReplyTxParams) (ProcessInboundReplyTxResult, error) {
	var result ProcessInboundReplyTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error

		// 1. Record the inbound message.
		result.Message, err = q.CreateMessage(ctx, arg.Message)
		if err != nil {
			return err
		}

		// 2. Advance collection_state and stamp last_response_at.
		newState := collectionStateForReply(arg.Classification)
		result.Invoice, err = q.SetInvoiceReplied(ctx, SetInvoiceRepliedParams{
			ID:              arg.InvoiceID,
			UserID:          arg.UserID,
			CollectionState: newState,
		})
		if err != nil {
			return err
		}

		// 3. Emit reply_received for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"classification":%q,"collection_state":%q}`,
			arg.Classification, newState,
		))
		result.Event, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     arg.UserID,
			CustomerID: arg.CustomerID,
			InvoiceID:  &arg.InvoiceID,
			EventType:  CustomerEventTypeReplyReceived,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
