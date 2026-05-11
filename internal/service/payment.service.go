package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"errors"
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

	return &dto.RecordPaymentResponse{
		Payment: dto.ToPaymentResponse(result.Payment),
		Invoice: dto.ToInvoiceResponse(result.Invoice),
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
