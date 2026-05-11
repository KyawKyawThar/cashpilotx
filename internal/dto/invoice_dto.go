package dto

import (
	db "cashpilotx/db/sqlc"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Invoice request DTOs
// =============================================================================

// CreateInvoiceRequest is the payload for POST /api/v1/invoices.
// Reminder fields (tone, channel, scheduled_at, max_attempts) configure the
// first reminder that is created atomically with the invoice.
type CreateInvoiceRequest struct {
	CustomerID  uuid.UUID       `json:"customer_id"  validate:"required"                        example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	Description *string         `json:"description"  validate:"omitempty,max=500"               example:"Web design services - May 2026"`
	Items       json.RawMessage `json:"items"        validate:"omitempty" swaggertype:"array,object"`
	Amount      int64           `json:"amount"       validate:"required,min=1"                  example:"150000"`
	Currency    string          `json:"currency"     validate:"required,len=3"                  example:"USD"`
	DueDate     time.Time       `json:"due_date"     validate:"required"                        example:"2026-06-01T00:00:00Z"`
	Source      *string         `json:"source"       validate:"omitempty,max=100"               example:"manual"`
	PhotoURL    *string         `json:"photo_url"    validate:"omitempty,url"                   example:"https://example.com/invoice.jpg"`

	// First reminder — all optional, defaults applied in service.
	Tone        string     `json:"tone"         validate:"omitempty,oneof=polite firm urgent"                        example:"polite"`
	Channel     string     `json:"channel"      validate:"omitempty,oneof=line whatsapp messenger wechat telegram viber"  example:"whatsapp"`
	ScheduledAt *time.Time `json:"scheduled_at" validate:"omitempty"                                                example:"2026-05-20T09:00:00Z"`
	MaxAttempts int32      `json:"max_attempts" validate:"omitempty,min=1,max=10"                                   example:"3"`
}

// MoveCollectionStateRequest is the payload for PATCH /api/v1/invoices/{id}/collection-state.
type MoveCollectionStateRequest struct {
	State string  `json:"state" validate:"required,oneof=pending reminded no_response negotiating promised paid defaulted"`
	Note  *string `json:"note"  validate:"omitempty,max=500"`
}

// ListInvoicesFilter holds optional filters for GET /api/v1/invoices.
type ListInvoicesFilter struct {
	Status          *string // unpaid | partial | paid | canceled
	Overdue         bool
	CollectionState *string // pending | reminded | no_response | negotiating | promised | paid | defaulted
	CustomerID      *uuid.UUID
}

// =============================================================================
// Invoice response DTOs
// =============================================================================

// InvoiceResponse is the public representation of an invoice.
type InvoiceResponse struct {
	ID                uuid.UUID        `json:"id"`
	CustomerID        uuid.UUID        `json:"customer_id"`
	ExternalID        *string          `json:"external_id,omitempty"`
	Description       *string          `json:"description,omitempty"`
	Items             json.RawMessage  `json:"items" swaggertype:"array,object"`
	Amount            int64            `json:"amount"`
	PaidAmount        int64            `json:"paid_amount"`
	Currency          string           `json:"currency"`
	Status            string           `json:"status"`
	Source            *string          `json:"source,omitempty"`
	PhotoURL          *string          `json:"photo_url,omitempty"`
	DueDate           time.Time        `json:"due_date"`
	IssuedAt          *time.Time       `json:"issued_at,omitempty"`
	PaidAt            *time.Time       `json:"paid_at,omitempty"`
	CollectionState   string           `json:"collection_state"`
	LastContactedAt   *time.Time       `json:"last_contacted_at,omitempty"`
	LastResponseAt    *time.Time       `json:"last_response_at,omitempty"`
	PromisedAt        *time.Time       `json:"promised_at,omitempty"`
	FollowUpCount     int32            `json:"follow_up_count"`
	ConfidenceScore   *float64         `json:"confidence_score,omitempty"`
	ConfidenceFactors *json.RawMessage `json:"confidence_factors,omitempty" swaggertype:"object"`
	OutcomeText       *string          `json:"outcome_text,omitempty"`
	OutcomeTimeframe  *string          `json:"outcome_timeframe,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// ReminderResponse is the public representation of a reminder.
type ReminderResponse struct {
	ID          uuid.UUID `json:"id"`
	InvoiceID   uuid.UUID `json:"invoice_id"`
	CustomerID  uuid.UUID `json:"customer_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Tone        string    `json:"tone"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	MaxAttempts int32     `json:"max_attempts"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateInvoiceResponse wraps the newly created invoice and its initial reminder.
type CreateInvoiceResponse struct {
	Invoice  *InvoiceResponse  `json:"invoice"`
	Reminder *ReminderResponse `json:"reminder"`
}

// =============================================================================
// Mapper functions
// =============================================================================

// ToInvoiceResponse maps db.Invoice to a client-facing InvoiceResponse.
func ToInvoiceResponse(i db.Invoice) *InvoiceResponse {
	items := i.Items
	if items == nil {
		items = json.RawMessage("[]")
	}
	return &InvoiceResponse{
		ID:                i.ID,
		CustomerID:        i.CustomerID,
		ExternalID:        i.ExternalID,
		Description:       i.Description,
		Items:             items,
		Amount:            i.Amount,
		PaidAmount:        i.PaidAmount,
		Currency:          i.Currency,
		Status:            i.Status,
		Source:            i.Source,
		PhotoURL:          i.PhotoUrl,
		DueDate:           i.DueDate,
		IssuedAt:          i.IssuedAt,
		PaidAt:            i.PaidAt,
		CollectionState:   i.CollectionState,
		LastContactedAt:   i.LastContactedAt,
		LastResponseAt:    i.LastResponseAt,
		PromisedAt:        i.PromisedAt,
		FollowUpCount:     i.FollowUpCount,
		ConfidenceScore:   i.ConfidenceScore,
		ConfidenceFactors: i.ConfidenceFactors,
		OutcomeText:       i.OutcomeText,
		OutcomeTimeframe:  i.OutcomeTimeframe,
		CreatedAt:         i.CreatedAt,
		UpdatedAt:         i.UpdatedAt,
	}
}

// ToReminderResponse maps db.Reminder to a client-facing ReminderResponse.
func ToReminderResponse(r db.Reminder) *ReminderResponse {
	return &ReminderResponse{
		ID:          r.ID,
		InvoiceID:   r.InvoiceID,
		CustomerID:  r.CustomerID,
		ScheduledAt: r.ScheduledAt,
		Tone:        r.Tone,
		Channel:     r.Channel,
		Status:      r.Status,
		MaxAttempts: r.MaxAttempts,
		CreatedAt:   r.CreatedAt,
	}
}
