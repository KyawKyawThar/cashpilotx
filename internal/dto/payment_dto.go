package dto

import (
	db "cashpilotx/db/sqlc"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Payment request DTOs
// =============================================================================

// RecordPaymentRequest is the payload for POST /api/v1/payments.
// Currency is intentionally absent — it is inherited from the invoice inside the transaction.
type RecordPaymentRequest struct {
	InvoiceID  uuid.UUID  `json:"invoice_id"  validate:"required"`
	CustomerID uuid.UUID  `json:"customer_id" validate:"required"`
	Amount     int64      `json:"amount"      validate:"required,min=1"`
	Method     string     `json:"method"      validate:"required,payment_method"`
	Reference  *string    `json:"reference"   validate:"omitempty,max=200"`
	Note       *string    `json:"note"        validate:"omitempty,max=500"`
	PaidOn     *time.Time `json:"paid_on"     validate:"omitempty"`
}

// VoidPaymentRequest is the payload for POST /api/v1/payments/{payment_id}/void.
type VoidPaymentRequest struct {
	CustomerID uuid.UUID `json:"customer_id" validate:"required"`
}

// =============================================================================
// Payment response DTOs
// =============================================================================

// PaymentResponse is the public representation of a single payment.
type PaymentResponse struct {
	ID        uuid.UUID  `json:"id"`
	InvoiceID uuid.UUID  `json:"invoice_id"`
	UserID    uuid.UUID  `json:"user_id"`
	PaidOn    time.Time  `json:"paid_on"`
	Amount    int64      `json:"amount"`
	Currency  string     `json:"currency"`
	Method    string     `json:"method"`
	Reference *string    `json:"reference,omitempty"`
	ProofURL  *string    `json:"proof_url,omitempty"`
	Note      *string    `json:"note,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	VoidedAt  *time.Time `json:"voided_at,omitempty"`
}

// RecordPaymentResponse wraps the new payment together with the updated invoice
// snapshot so the client can update its local state in a single round-trip.
type RecordPaymentResponse struct {
	Payment *PaymentResponse `json:"payment"`
	Invoice *InvoiceResponse `json:"invoice"`
}

type PaymentListItem struct {
	*PaymentResponse
	CustomerName  string `json:"customer_name"`
	InvoiceNumber string `json:"invoice_number"`
	IsFullPayment bool   `json:"is_full_payment"` // invoice_status == "paid"
}

// =============================================================================
// Mapper functions
// =============================================================================

// ToPaymentResponse maps db.Payment to a client-facing PaymentResponse.
func ToPaymentResponse(p db.Payment) *PaymentResponse {
	return &PaymentResponse{
		ID:        p.ID,
		InvoiceID: p.InvoiceID,
		UserID:    p.UserID,
		PaidOn:    p.PaidOn,
		Amount:    p.Amount,
		Currency:  p.Currency,
		Method:    p.Method,
		Reference: p.Reference,
		ProofURL:  p.ProofUrl,
		Note:      p.Note,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		VoidedAt:  p.VoidedAt,
	}
}

func assemblePaymentListItem(
	resp *PaymentResponse,
	customerName string,
	invoiceNumber *string,
	invoiceStatus string,
) *PaymentListItem {
	invNum := ""
	if invoiceNumber != nil {
		invNum = *invoiceNumber
	}
	return &PaymentListItem{
		PaymentResponse: resp,
		CustomerName:    customerName,
		InvoiceNumber:   invNum,
		IsFullPayment:   invoiceStatus == "paid",
	}
}

// ToPaymentListItem maps a ListPaymentsEnrichedRow to the enriched list response.
func ToPaymentListItem(row db.ListPaymentsEnrichedRow) *PaymentListItem {
	return assemblePaymentListItem(
		&PaymentResponse{
			ID:        row.ID,
			InvoiceID: row.InvoiceID,
			UserID:    row.UserID,
			PaidOn:    row.PaidOn,
			Amount:    row.Amount,
			Currency:  row.Currency,
			Method:    row.Method,
			Reference: row.Reference,
			ProofURL:  row.ProofUrl,
			Note:      row.Note,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			VoidedAt:  row.VoidedAt,
		},
		row.CustomerName,
		row.InvoiceNumber,
		row.InvoiceStatus,
	)
}

// ToPaymentListItemFromSearch maps a SearchPaymentsEnrichedRow to the enriched list response.
func ToPaymentListItemFromSearch(row db.SearchPaymentsEnrichedRow) *PaymentListItem {
	return assemblePaymentListItem(
		&PaymentResponse{
			ID:        row.ID,
			InvoiceID: row.InvoiceID,
			UserID:    row.UserID,
			PaidOn:    row.PaidOn,
			Amount:    row.Amount,
			Currency:  row.Currency,
			Method:    row.Method,
			Reference: row.Reference,
			ProofURL:  row.ProofUrl,
			Note:      row.Note,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			VoidedAt:  row.VoidedAt,
		},
		row.CustomerName,
		row.InvoiceNumber,
		row.InvoiceStatus,
	)
}

// ToPaymentListItemToday maps a ListPaymentsEnrichedTodayRow to the enriched list response.
func ToPaymentListItemToday(row db.ListPaymentsEnrichedTodayRow) *PaymentListItem {
	return assemblePaymentListItem(
		&PaymentResponse{
			ID:        row.ID,
			InvoiceID: row.InvoiceID,
			UserID:    row.UserID,
			PaidOn:    row.PaidOn,
			Amount:    row.Amount,
			Currency:  row.Currency,
			Method:    row.Method,
			Reference: row.Reference,
			ProofURL:  row.ProofUrl,
			Note:      row.Note,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			VoidedAt:  row.VoidedAt,
		},
		row.CustomerName,
		row.InvoiceNumber,
		row.InvoiceStatus,
	)
}
