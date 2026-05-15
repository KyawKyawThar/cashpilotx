package db

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UpdateReminderParams are the editable fields for a pending reminder.
type UpdateReminderParams struct {
	ID          uuid.UUID `db:"id"           json:"id"`
	ScheduledAt time.Time `db:"scheduled_at" json:"scheduled_at"`
	Tone        string    `db:"tone"         json:"tone"`
	Channel     string    `db:"channel"      json:"channel"`
	MaxAttempts int32     `db:"max_attempts" json:"max_attempts"`
}

const updateReminderSQL = `
UPDATE reminders
SET scheduled_at = $2,
    tone         = $3,
    channel      = $4,
    max_attempts = $5,
    updated_at   = now()
WHERE id     = $1
  AND status = 'pending'
RETURNING id, user_id, invoice_id, customer_id, scheduled_at, tone, channel, status,
          attempt_number, max_attempts, last_error, next_retry_at, external_message_id,
          sent_at, cancelled_at, created_at, updated_at
`

// UpdateReminder edits the schedule/tone/channel of a pending reminder.
// Returns pgx.ErrNoRows (→ 404) when the reminder is not found or not in pending state.
func (s *SQLStore) UpdateReminder(ctx context.Context, arg UpdateReminderParams) (Reminder, error) {
	row := s.connPool.QueryRow(ctx, updateReminderSQL,
		arg.ID,
		arg.ScheduledAt,
		arg.Tone,
		arg.Channel,
		arg.MaxAttempts,
	)
	var i Reminder
	err := row.Scan(
		&i.ID,
		&i.UserID,
		&i.InvoiceID,
		&i.CustomerID,
		&i.ScheduledAt,
		&i.Tone,
		&i.Channel,
		&i.Status,
		&i.AttemptNumber,
		&i.MaxAttempts,
		&i.LastError,
		&i.NextRetryAt,
		&i.ExternalMessageID,
		&i.SentAt,
		&i.CancelledAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}
