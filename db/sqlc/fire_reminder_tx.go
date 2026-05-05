package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// FireReminderTx atomically marks a reminder as sent, records the outbound
// message, advances the invoice collection_state to 'reminded', increments
// follow_up_count, and emits a reminder_sent event for the History timeline.
// Optionally schedules the next reminder when more attempts remain.
//
// Tone escalation rule (caller sets the tone on arg.Message):
//   follow_up_count == 0 → friendly/polite
//   follow_up_count == 1 → firm
//   follow_up_count >= 2 → strict/urgent

type FireReminderTxParams struct {
	ReminderUpdate MarkReminderSentParams
	Message        CreateMessageParams
	NextReminder   *CreateReminderParams // nil → no follow-up scheduled

	// Required to advance collection_state and emit the timeline event.
	InvoiceID  uuid.UUID
	UserID     uuid.UUID
	CustomerID uuid.UUID
}

type FireReminderTxResult struct {
	Invoice      Invoice
	Message      Message
	NextReminder *Reminder
}

func (s *SQLStore) FireReminderTx(ctx context.Context, arg FireReminderTxParams) (FireReminderTxResult, error) {
	var result FireReminderTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		// 1. Mark the reminder sent (status: processing → sent).
		if err := q.MarkReminderSent(ctx, arg.ReminderUpdate); err != nil {
			return err
		}

		// 2. Record the outbound message.
		var err error
		result.Message, err = q.CreateMessage(ctx, arg.Message)
		if err != nil {
			return err
		}

		// 3. Advance collection_state to 'reminded', stamp last_contacted_at,
		//    and increment follow_up_count.
		result.Invoice, err = q.SetInvoiceContacted(ctx, SetInvoiceContactedParams{
			ID:              arg.InvoiceID,
			UserID:          arg.UserID,
			CollectionState: "reminded",
		})
		if err != nil {
			return err
		}

		// 4. Schedule next reminder if the caller supplied one.
		if arg.NextReminder != nil {
			var next Reminder
			next, err = q.CreateReminder(ctx, *arg.NextReminder)
			if err != nil {
				return err
			}
			result.NextReminder = &next
		}

		// 5. Emit reminder_sent for the History timeline.
		payload := json.RawMessage(fmt.Sprintf(
			`{"channel":%q,"tone":%q,"follow_up_count":%d}`,
			arg.Message.Channel, arg.Message.Tone, result.Invoice.FollowUpCount,
		))
		_, err = q.CreateCustomerEvent(ctx, CreateCustomerEventParams{
			UserID:     arg.UserID,
			CustomerID: arg.CustomerID,
			InvoiceID:  &arg.InvoiceID,
			EventType:  CustomerEventTypeReminderSent,
			EventValue: &payload,
		})
		return err
	})

	return result, err
}
