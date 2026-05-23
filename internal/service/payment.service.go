package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RecordPayment runs RecordPaymentTx: inserts the payment, recomputes invoice
// status from the payments table, flips collection_state to 'paid' when fully
// settled, updates the customer's running totals, and emits a payment_received
// event — all in a single atomic transaction.
func (s *PaymentService) RecordPayment(
	ctx context.Context,
	userID uuid.UUID,
	req *dto.RecordPaymentRequest,
) (*dto.RecordPaymentResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	paidOn := time.Now()
	if req.PaidOn != nil {
		paidOn = *req.PaidOn
	}

	result, err := s.store.RecordPaymentTx(dbCtx, db.RecordPaymentTxParams{
		Payment: db.CreatePaymentParams{
			InvoiceID: req.InvoiceID,
			UserID:    userID,
			Amount:    req.Amount,
			Method:    req.Method,
			Reference: req.Reference,
			Note:      req.Note,
			PaidOn:    paidOn,
		},
		CustomerID: req.CustomerID,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrPaymentAmountInvalid):
			return nil, api.ErrBadRequest("payment amount must be positive")
		case errors.Is(err, db.ErrPaymentExceedsBalance):
			return nil, api.ErrInvoiceOverpaid()
		case api.IsRecordNotFound(err):
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	// Write audit log asynchronously — must not block or fail the request.
	isFull := result.Invoice.PaidAmount >= result.Invoice.Amount
	sym := "฿"
	if result.Invoice.Currency == "MMK" {
		sym = "K"
	}
	qualifier := "partial"
	if isFull {
		qualifier = "full"
	}
	detail := fmt.Sprintf(
		"Recorded %s payment of %s%d via %s.",
		qualifier, sym, result.Payment.Amount, result.Payment.Method,
	)
	meta, err := json.Marshal(map[string]any{
		"method":          result.Payment.Method,
		"amount":          result.Payment.Amount,
		"currency":        result.Invoice.Currency,
		"is_full_payment": isFull,
	})
	if err != nil {
		meta = nil
	}
	invID := result.Invoice.ID
	custID := result.Customer.ID
	payID := result.Payment.ID
	logAuditAsync(ctx, s.store, db.CreateOperatorAuditLogParams{
		UserID:        result.Invoice.UserID,
		PerformedBy:   result.Invoice.UserID,
		ActionType:    db.AuditActionTypePaymentRecorded,
		EntityType:    db.AuditEntityTypePayment,
		ActionLabel:   "Payment Recorded",
		Detail:        detail,
		InvoiceID:     &invID,
		InvoiceNumber: result.Invoice.ExternalID,
		CustomerID:    &custID,
		PaymentID:     &payID,
		Metadata:      meta,
	})

	return &dto.RecordPaymentResponse{
		Payment: dto.ToPaymentResponse(result.Payment),
		Invoice: dto.ToInvoiceResponse(result.Invoice, nil),
	}, nil
}

// GetPayment returns a single payment owned by userID.
func (s *PaymentService) GetPayment(
	ctx context.Context,
	userID, paymentID uuid.UUID,
) (*dto.PaymentResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	p, err := s.store.GetPaymentByID(dbCtx, db.GetPaymentByIDParams{
		ID:     paymentID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrPaymentNotFound()
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToPaymentResponse(p), nil
}

// ListPayments returns a paginated, enriched list of payments for userID,
// including customer name, invoice number, and whether the invoice was fully settled.
//
//nolint:dupl // identical paginated-list pattern shared across services; abstraction adds no value
func (s *PaymentService) ListPayments(
	ctx context.Context,
	userID uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.PaymentListItem, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListPaymentsEnriched(dbCtx, db.ListPaymentsEnrichedParams{
		UserID: userID,
		Limit:  pg.Limit32(),
		Offset: pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.PaymentListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToPaymentListItem(r))
	}
	return out, nil
}

// VoidPayment marks a payment as voided, recomputes the invoice status from the
// remaining non-voided payments, and updates the customer running totals.
func (s *PaymentService) VoidPayment(
	ctx context.Context,
	userID, paymentID uuid.UUID,
	customerID uuid.UUID,
) (*dto.PaymentResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	result, err := s.store.VoidPaymentTx(dbCtx, db.VoidPaymentTxParams{
		PaymentID:  paymentID,
		UserID:     userID,
		CustomerID: customerID,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrPaymentAlreadyVoided):
			return nil, api.ErrBadRequest("payment is already voided")
		case api.IsRecordNotFound(err):
			return nil, api.ErrPaymentNotFound()
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToPaymentResponse(result.Payment), nil
}

// ListPaymentsByInvoice returns all payments for a specific invoice owned by userID.
func (s *PaymentService) ListPaymentsByInvoice(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
) ([]*dto.PaymentResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListPaymentsByInvoice(dbCtx, db.ListPaymentsByInvoiceParams{
		UserID:    userID,
		InvoiceID: invoiceID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.PaymentResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToPaymentResponse(r))
	}
	return out, nil
}
