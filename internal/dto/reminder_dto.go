package dto

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Reminder request DTOs
// =============================================================================

// CreateReminderRequest is the payload for POST /api/v1/invoices/{id}/reminders.
type CreateReminderRequest struct {
	CustomerID  uuid.UUID `json:"customer_id"  validate:"required"`
	ScheduledAt time.Time `json:"scheduled_at" validate:"required"`
	Tone        string    `json:"tone"         validate:"omitempty,oneof=polite firm urgent final"`
	Channel     string    `json:"channel"      validate:"omitempty,oneof=line whatsapp messenger wechat telegram viber"`
	MaxAttempts int32     `json:"max_attempts" validate:"omitempty,min=1,max=10"`
}

// MarkReminderSentRequest is the payload for POST /api/v1/reminders/{id}/mark-sent.
type MarkReminderSentRequest struct {
	ExternalMessageID *string `json:"external_message_id" validate:"omitempty,max=200"`
}
