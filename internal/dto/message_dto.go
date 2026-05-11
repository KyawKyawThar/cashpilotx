package dto

import (
	db "cashpilotx/db/sqlc"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Message request DTOs
// =============================================================================

// CreateManualMessageRequest is the payload for POST /api/v1/messages/manual.
type CreateManualMessageRequest struct {
	InvoiceID  uuid.UUID `json:"invoice_id"  validate:"required"`
	CustomerID uuid.UUID `json:"customer_id" validate:"required"`
	Channel    string    `json:"channel"     validate:"required,oneof=line whatsapp messenger wechat telegram viber external"`
	Content    string    `json:"content"     validate:"required,min=1,max=4000"`
	Tone       string    `json:"tone"        validate:"omitempty,oneof=polite firm urgent final"`
}

// MarkMessageSentRequest is the payload for POST /api/v1/messages/{id}/mark-sent.
type MarkMessageSentRequest struct {
	ExternalMessageID *string `json:"external_message_id" validate:"omitempty,max=200"`
}

// =============================================================================
// Message response DTO
// =============================================================================

type ChatMessageResponse struct {
	ID                uuid.UUID       `json:"id"`
	InvoiceID         *uuid.UUID      `json:"invoice_id,omitempty"`
	CustomerID        uuid.UUID       `json:"customer_id"`
	Direction         string          `json:"direction"`
	Channel           string          `json:"channel"`
	Content           string          `json:"content"`
	Tone              string          `json:"tone"`
	Status            string          `json:"status"`
	Source            string          `json:"source"`
	ExternalMessageID *string         `json:"external_message_id,omitempty"`
	Meta              json.RawMessage `json:"meta" swaggertype:"object"`
	SentAt            *time.Time      `json:"sent_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

func ToChatMessageResponse(m db.Message) *ChatMessageResponse {
	return &ChatMessageResponse{
		ID:                m.ID,
		InvoiceID:         m.InvoiceID,
		CustomerID:        m.CustomerID,
		Direction:         m.Direction,
		Channel:           m.Channel,
		Content:           m.Content,
		Tone:              m.Tone,
		Status:            m.Status,
		Source:            m.Source,
		ExternalMessageID: m.ExternalMessageID,
		Meta:              m.Meta,
		SentAt:            m.SentAt,
		CreatedAt:         m.CreatedAt,
	}
}

// CopyTextResponse is the payload returned by GET /api/v1/messages/{id}/copy-text.
type CopyTextResponse struct {
	MessageID uuid.UUID `json:"message_id"`
	Content   string    `json:"content"`
}
