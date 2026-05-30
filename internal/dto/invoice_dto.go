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
	CustomerID      uuid.UUID       `json:"customer_id"  validate:"required"                        example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	Description     *string         `json:"description"  validate:"omitempty,max=500"               example:"Web design services - May 2026"`
	Items           json.RawMessage `json:"items"        validate:"omitempty" swaggertype:"array,object"`
	Amount          int64           `json:"amount"       validate:"required,min=1"                  example:"150000"`
	Currency        string          `json:"currency"     validate:"required,len=3"                  example:"USD"`
	DueDate         time.Time       `json:"due_date"     validate:"required"                        example:"2026-06-01T00:00:00Z"`
	Source          *string         `json:"source"       validate:"omitempty,max=100"               example:"manual"`
	PhotoURL        *string         `json:"photo_url"    validate:"omitempty,url"                   example:"https://example.com/invoice.jpg"`
	AssignedTo      *uuid.UUID      `json:"assigned_to"       validate:"omitempty"`
	DeliveryService *string         `json:"delivery_service"  validate:"omitempty,max=100"   example:"Flash Express"`
	TrackingNumber  *string         `json:"tracking_number"   validate:"omitempty,max=100"   example:"10661980LZR"`

	// First reminder — all optional, defaults applied in service.
	Tone        string     `json:"tone"         validate:"omitempty,oneof=polite firm urgent"                        example:"polite"`
	Channel     string     `json:"channel"      validate:"omitempty,oneof=line whatsapp messenger wechat telegram viber"  example:"whatsapp"`
	ScheduledAt *time.Time `json:"scheduled_at" validate:"omitempty"                                                example:"2026-05-20T09:00:00Z"`
	MaxAttempts int32      `json:"max_attempts" validate:"omitempty,min=1,max=10"                                   example:"3"`
}

// CreateDeliveryServiceRequest is the payload for POST /api/v1/delivery-services.
type CreateDeliveryServiceRequest struct {
	Name     string `json:"name"     validate:"required,min=1,max=100"`
	Price    int64  `json:"price"    validate:"min=0"`
	Currency string `json:"currency" validate:"required,len=3"`
}

// MoveCollectionStateRequest is the payload for PATCH /api/v1/invoices/{id}/collection-state.
type MoveCollectionStateRequest struct {
	State      string     `json:"state"       validate:"required,oneof=pending reminded no_response negotiating promised verifying paid defaulted"`
	Note       *string    `json:"note"        validate:"omitempty,max=500"`
	PromisedAt *time.Time `json:"promised_at"` // only honored when State == "promised"
}

// UpdateInvoiceRequest is the payload for PATCH /api/v1/invoices/{id}.
// Only allowed when the invoice has no recorded payments (paid_amount == 0).
type UpdateInvoiceRequest struct {
	Items           json.RawMessage `json:"items"            validate:"omitempty" swaggertype:"array,object"`
	Amount          int64           `json:"amount"           validate:"required,min=1"`
	Currency        string          `json:"currency"         validate:"required,len=3"`
	DueDate         time.Time       `json:"due_date"         validate:"required"`
	Description     *string         `json:"description"      validate:"omitempty,max=500"`
	AssignedTo      *uuid.UUID      `json:"assigned_to"      validate:"omitempty"`
	DeliveryService *string         `json:"delivery_service" validate:"omitempty,max=100"`
	TrackingNumber  *string         `json:"tracking_number"  validate:"omitempty,max=100"`
}

// UpdateInvoiceTrackingRequest is the payload for PATCH /api/v1/invoices/{id}/tracking.
type UpdateInvoiceTrackingRequest struct {
	TrackingNumber *string `json:"tracking_number" validate:"omitempty,max=100" example:"10661980LZR"`
}

// ListInvoicesFilter holds optional filters for GET /api/v1/invoices.
type ListInvoicesFilter struct {
	Status           *string // unpaid | partial | paid | canceled
	Overdue          bool
	CollectionState  *string // pending | reminded | no_response | negotiating | promised | verifying | paid | defaulted
	CustomerID       *uuid.UUID
	AssignedTo       *uuid.UUID // filter by assignee user ID — used for per-rep reporting
	CurrentMonthOnly bool       // enforced for sales role by scope middleware
	Search           *string    // full-text: customer name/phone, tracking, external_id
}

// =============================================================================
// Invoice response DTOs
// =============================================================================

// InvoiceResponse is the public representation of an invoice.
type InvoiceResponse struct {
	ID                 uuid.UUID        `json:"id"`
	CustomerID         uuid.UUID        `json:"customer_id"`
	ExternalID         *string          `json:"external_id,omitempty"`
	Description        *string          `json:"description,omitempty"`
	Items              json.RawMessage  `json:"items" swaggertype:"array,object"`
	Amount             int64            `json:"amount"`
	PaidAmount         int64            `json:"paid_amount"`
	Currency           string           `json:"currency"`
	Status             string           `json:"status"`
	Source             *string          `json:"source,omitempty"`
	PhotoURL           *string          `json:"photo_url,omitempty"`
	DueDate            time.Time        `json:"due_date"`
	IssuedAt           *time.Time       `json:"issued_at,omitempty"`
	PaidAt             *time.Time       `json:"paid_at,omitempty"`
	CollectionState    string           `json:"collection_state"`
	LastContactedAt    *time.Time       `json:"last_contacted_at,omitempty"`
	LastResponseAt     *time.Time       `json:"last_response_at,omitempty"`
	PromisedAt         *time.Time       `json:"promised_at,omitempty"`
	FollowUpCount      int32            `json:"follow_up_count"`
	ConfidenceScore    *float64         `json:"confidence_score,omitempty"`
	ConfidenceFactors  *json.RawMessage `json:"confidence_factors,omitempty" swaggertype:"object"`
	OutcomeText        *string          `json:"outcome_text,omitempty"`
	OutcomeTimeframe   *string          `json:"outcome_timeframe,omitempty"`
	AssignedTo         *uuid.UUID       `json:"assigned_to,omitempty"`
	AssignedToUsername *string          `json:"assigned_to_username,omitempty"`
	DeliveryService    *string          `json:"delivery_service,omitempty"`
	TrackingNumber     *string          `json:"tracking_number,omitempty"`
	ReturnedAt         *time.Time       `json:"returned_at,omitempty"`
	ReturnReason       *string          `json:"return_reason,omitempty"`
	ReturnItems        json.RawMessage  `json:"return_items,omitempty" swaggertype:"array,object"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	// Customer fields — populated on list endpoints to avoid a separate /customers fetch.
	CustomerName   *string `json:"customer_name,omitempty"`
	CustomerPhone  *string `json:"customer_phone,omitempty"`
	CustomerNumber *string `json:"customer_number,omitempty"`
}

// ReturnInvoiceRequest is the optional payload for POST /api/v1/invoices/{id}/return.
type ReturnInvoiceRequest struct {
	ReturnReason *string         `json:"return_reason" validate:"omitempty,oneof=damaged wrong_item customer_rejected change_of_mind other"`
	ReturnItems  json.RawMessage `json:"return_items"  validate:"omitempty" swaggertype:"array,object"`
	SoldBy       *uuid.UUID      `json:"sold_by"       validate:"omitempty"`
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
// Pass assigneeName as the resolved username for the assigned_to UUID; nil if unknown or unassigned.
func ToInvoiceResponse(i db.Invoice, assigneeName *string) *InvoiceResponse {
	items := i.Items
	if items == nil {
		items = json.RawMessage("[]")
	}
	return &InvoiceResponse{
		ID:                 i.ID,
		CustomerID:         i.CustomerID,
		ExternalID:         i.ExternalID,
		Description:        i.Description,
		Items:              items,
		Amount:             i.Amount,
		PaidAmount:         i.PaidAmount,
		Currency:           i.Currency,
		Status:             i.Status,
		Source:             i.Source,
		PhotoURL:           i.PhotoUrl,
		DueDate:            i.DueDate,
		IssuedAt:           i.IssuedAt,
		PaidAt:             i.PaidAt,
		CollectionState:    i.CollectionState,
		LastContactedAt:    i.LastContactedAt,
		LastResponseAt:     i.LastResponseAt,
		PromisedAt:         i.PromisedAt,
		FollowUpCount:      i.FollowUpCount,
		ConfidenceScore:    i.ConfidenceScore,
		ConfidenceFactors:  i.ConfidenceFactors,
		OutcomeText:        i.OutcomeText,
		OutcomeTimeframe:   i.OutcomeTimeframe,
		AssignedTo:         i.AssignedTo,
		AssignedToUsername: assigneeName,
		DeliveryService:    i.DeliveryService,
		TrackingNumber:     i.TrackingNumber,
		ReturnedAt:         i.ReturnedAt,
		ReturnReason:       i.ReturnReason,
		ReturnItems: func() json.RawMessage {
			if i.ReturnItems == nil {
				return nil
			}
			return *i.ReturnItems
		}(),
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
	}
}

// ToInvoiceResponseFromSearch maps a SearchInvoicesRow (invoice + embedded customer) to InvoiceResponse.
func ToInvoiceResponseFromSearch(r db.SearchInvoicesRow, assigneeName *string) *InvoiceResponse {
	items := r.Items
	if items == nil {
		items = json.RawMessage("[]")
	}
	resp := &InvoiceResponse{
		ID:                 r.ID,
		CustomerID:         r.CustomerID,
		ExternalID:         r.ExternalID,
		Description:        r.Description,
		Items:              items,
		Amount:             r.Amount,
		PaidAmount:         r.PaidAmount,
		Currency:           r.Currency,
		Status:             r.Status,
		Source:             r.Source,
		PhotoURL:           r.PhotoUrl,
		DueDate:            r.DueDate,
		IssuedAt:           r.IssuedAt,
		PaidAt:             r.PaidAt,
		CollectionState:    r.CollectionState,
		LastContactedAt:    r.LastContactedAt,
		LastResponseAt:     r.LastResponseAt,
		PromisedAt:         r.PromisedAt,
		FollowUpCount:      r.FollowUpCount,
		ConfidenceScore:    r.ConfidenceScore,
		ConfidenceFactors:  r.ConfidenceFactors,
		OutcomeText:        r.OutcomeText,
		OutcomeTimeframe:   r.OutcomeTimeframe,
		AssignedTo:         r.AssignedTo,
		AssignedToUsername: assigneeName,
		DeliveryService:    r.DeliveryService,
		TrackingNumber:     r.TrackingNumber,
		ReturnedAt:         r.ReturnedAt,
		ReturnReason:       r.ReturnReason,
		ReturnItems: func() json.RawMessage {
			if r.ReturnItems == nil {
				return nil
			}
			return *r.ReturnItems
		}(),
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
		CustomerName:   &r.CustomerName,
		CustomerPhone:  r.CustomerPhone,
		CustomerNumber: r.CustomerNumber,
	}
	return resp
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
