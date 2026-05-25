package dto

import (
	db "cashpilotx/db/sqlc"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Dashboard
// =============================================================================

type DashboardResponse struct {
	TotalRecoverable   int64 `json:"total_recoverable"`
	WaitingForReply    int64 `json:"waiting_for_reply"`
	OpenActions        int32 `json:"open_actions"`
	RemindedCount      int32 `json:"reminded_count"`
	CollectedThisMonth int64 `json:"collected_this_month"`
	TotalMessagesSent  int32 `json:"total_messages_sent"`
	OpenInvoicesCount  int32 `json:"open_invoices_count"`
	CollectionRate     int32 `json:"collection_rate"` // 0–100, total_collected / total_invoiced
}

// CollectedSummaryResponse is returned by GET /api/v1/payments/summary.
type CollectedSummaryResponse struct {
	Collected    int64  `json:"collected"`
	PaymentCount int32  `json:"payment_count"`
	From         string `json:"from"`
	To           string `json:"to"`
}

// =============================================================================
// Recovery Queue
// =============================================================================

type RecoveryQueueItemResponse struct {
	ID               uuid.UUID  `json:"id"`
	CustomerID       uuid.UUID  `json:"customer_id"`
	CustomerName     string     `json:"customer_name"`
	CustomerPhone    *string    `json:"customer_phone,omitempty"`
	ExternalID       *string    `json:"external_id,omitempty"`
	TrackingNumber   *string    `json:"tracking_number,omitempty"`
	Amount           int64      `json:"amount"`
	PaidAmount       int64      `json:"paid_amount"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	DueDate          time.Time  `json:"due_date"`
	CollectionState  string     `json:"collection_state"`
	ConfidenceScore  *float64   `json:"confidence_score,omitempty"`
	OutcomeText      *string    `json:"outcome_text,omitempty"`
	OutcomeTimeframe *string    `json:"outcome_timeframe,omitempty"`
	LastContactedAt  *time.Time `json:"last_contacted_at,omitempty"`
	FollowUpCount    int32      `json:"follow_up_count"`
	CreatedAt        time.Time  `json:"created_at"`
}

func ToRecoveryQueueItemResponse(row db.GetRecoveryQueueRow) *RecoveryQueueItemResponse {
	return &RecoveryQueueItemResponse{
		ID:               row.ID,
		CustomerID:       row.CustomerID,
		CustomerName:     row.CustomerName,
		CustomerPhone:    row.CustomerPhone,
		ExternalID:       row.ExternalID,
		TrackingNumber:   row.TrackingNumber,
		Amount:           row.Amount,
		PaidAmount:       row.PaidAmount,
		Currency:         row.Currency,
		Status:           row.Status,
		DueDate:          row.DueDate,
		CollectionState:  row.CollectionState,
		ConfidenceScore:  row.ConfidenceScore,
		OutcomeText:      row.OutcomeText,
		OutcomeTimeframe: row.OutcomeTimeframe,
		LastContactedAt:  row.LastContactedAt,
		FollowUpCount:    row.FollowUpCount,
		CreatedAt:        row.CreatedAt,
	}
}

// =============================================================================
// Customer Events
// =============================================================================

type CustomerEventResponse struct {
	ID         uuid.UUID        `json:"id"`
	CustomerID uuid.UUID        `json:"customer_id"`
	InvoiceID  *uuid.UUID       `json:"invoice_id,omitempty"`
	EventType  string           `json:"event_type"`
	EventValue *json.RawMessage `json:"event_value,omitempty" swaggertype:"object"`
	CreatedAt  time.Time        `json:"created_at"`
}

func ToCustomerEventResponse(e db.CustomerEvent) *CustomerEventResponse {
	return &CustomerEventResponse{
		ID:         e.ID,
		CustomerID: e.CustomerID,
		InvoiceID:  e.InvoiceID,
		EventType:  string(e.EventType),
		EventValue: e.EventValue,
		CreatedAt:  e.CreatedAt,
	}
}

// =============================================================================
// Audit Log
// =============================================================================

type AuditLogResponse struct {
	ID            uuid.UUID       `json:"id"`
	PerformedBy   uuid.UUID       `json:"performed_by"`
	ActionType    string          `json:"action_type"`
	EntityType    string          `json:"entity_type"`
	ActionLabel   string          `json:"action_label"`
	Detail        string          `json:"detail"`
	InvoiceID     *uuid.UUID      `json:"invoice_id,omitempty"`
	InvoiceNumber *string         `json:"invoice_number,omitempty"`
	CustomerID    *uuid.UUID      `json:"customer_id,omitempty"`
	PaymentID     *uuid.UUID      `json:"payment_id,omitempty"`
	ReminderID    *uuid.UUID      `json:"reminder_id,omitempty"`
	MessageID     *uuid.UUID      `json:"message_id,omitempty"`
	IsReverted    bool            `json:"is_reverted"`
	RevertedBy    *uuid.UUID      `json:"reverted_by,omitempty"`
	RevertedAt    *time.Time      `json:"reverted_at,omitempty"`
	Metadata      json.RawMessage `json:"metadata" swaggertype:"object"`
	CreatedAt     time.Time       `json:"created_at"`
}

func ToAuditLogResponse(a db.OperatorAuditLog) *AuditLogResponse {
	return &AuditLogResponse{
		ID:            a.ID,
		PerformedBy:   a.PerformedBy,
		ActionType:    string(a.ActionType),
		EntityType:    string(a.EntityType),
		ActionLabel:   a.ActionLabel,
		Detail:        a.Detail,
		InvoiceID:     a.InvoiceID,
		InvoiceNumber: a.InvoiceNumber,
		CustomerID:    a.CustomerID,
		PaymentID:     a.PaymentID,
		ReminderID:    a.ReminderID,
		MessageID:     a.MessageID,
		IsReverted:    a.IsReverted,
		RevertedBy:    a.RevertedBy,
		RevertedAt:    a.RevertedAt,
		Metadata:      a.Metadata,
		CreatedAt:     a.CreatedAt,
	}
}
