package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// MoveCollectionStateTx atomically advances an invoice's collection_state
// via a manual owner override (the "Move state" modal in v3 UI), and emits
// a collection_state_changed event for the History timeline.

type MoveCollectionStateTxParams struct {
	InvoiceID       uuid.UUID
	UserID          uuid.UUID
	CustomerID      uuid.UUID
	CollectionState string  // target state
	Note            *string // optional owner note stored in the event payload
}

type MoveCollectionStateTxResult struct {
	Invoice Invoice
	Event   CustomerEvent
}

func (s *SQLStore) MoveCollectionStateTx(ctx context.Context, arg MoveCollectionStateTxParams) (MoveCollectionStateTxResult, error) {
	var result MoveCollectionStateTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error

		// 1. Apply the new state.
		result.Invoice, err = q.UpdateInvoiceCollectionState(ctx, UpdateInvoiceCollectionStateParams{
			ID:              arg.InvoiceID,
			UserID:          arg.UserID,
			CollectionState: arg.CollectionState,
		})
		if err != nil {
			return err
		}

		// 2. Emit collection_state_changed for the History timeline.
		var payloadStr string
		if arg.Note != nil {
			payloadStr = fmt.Sprintf(
				`{"to":%q,"note":%q}`,
				arg.CollectionState, *arg.Note,
			)
		} else {
			payloadStr = fmt.Sprintf(`{"to":%q}`, arg.CollectionState)
		}
		payload := json.RawMessage(payloadStr)
		result.Event, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     arg.UserID,
			CustomerID: arg.CustomerID,
			InvoiceID:  &arg.InvoiceID,
			EventType:  CustomerEventTypeCollectionStateChanged,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
