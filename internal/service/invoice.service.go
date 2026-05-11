package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// validTransitions maps each collection_state to the states it may advance to.
// Terminal states (paid, defaulted) are not present as keys — no further transitions.
var validTransitions = map[string][]string{
	"pending":     {"reminded", "paid", "defaulted"},
	"reminded":    {"no_response", "negotiating", "paid", "defaulted"},
	"no_response": {"reminded", "paid", "defaulted"},
	"negotiating": {"promised", "paid", "defaulted"},
	"promised":    {"paid", "defaulted"},
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// formatExternalID returns the invoice number in INV-YYYY-NNN format.
func formatExternalID(seq int32) string {
	return fmt.Sprintf("INV-%d-%03d", time.Now().Year(), seq)
}

// CreateInvoice atomically creates an invoice and its first reminder, generating
// the external_id (INV-YYYY-NNN) via the user's atomic invoice_sequence counter.
func (s *InvoiceService) CreateInvoice(
	ctx context.Context,
	userID uuid.UUID,
	req *dto.CreateInvoiceRequest,
) (*dto.CreateInvoiceResponse, error) {
	tone := req.Tone
	if tone == "" {
		tone = defaultTone
	}
	channel := req.Channel
	if channel == "" {
		channel = "line"
	}
	scheduledAt := req.DueDate
	if req.ScheduledAt != nil {
		scheduledAt = *req.ScheduledAt
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}

	items := req.Items
	if len(items) == 0 {
		items = []byte("[]")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	// Validate customer ownership before burning a sequence slot.
	if _, err := s.store.GetCustomerByID(dbCtx, db.GetCustomerByIDParams{
		ID:     req.CustomerID,
		UserID: userID,
	}); err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrCustomerNotFound()
		}
		return nil, api.FromPgError(err)
	}

	// Atomically increment the sequence; the returned number forms the external_id.
	// If CreateInvoiceTx later fails, the sequence number is burned (gap is acceptable).
	seq, err := s.store.NextInvoiceSequence(dbCtx, userID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}
	externalID := formatExternalID(seq)

	result, err := s.store.CreateInvoiceTx(dbCtx, db.CreateInvoiceTxParams{
		Invoice: db.CreateInvoiceParams{
			UserID:      userID,
			CustomerID:  req.CustomerID,
			Amount:      req.Amount,
			Currency:    req.Currency,
			Description: req.Description,
			DueDate:     req.DueDate,
			Source:      req.Source,
			PhotoUrl:    req.PhotoURL,
			ExternalID:  &externalID,
			Column10:    items,
		},
		Reminder: db.CreateReminderParams{
			UserID:      userID,
			ScheduledAt: scheduledAt,
			Tone:        tone,
			Channel:     channel,
			MaxAttempts: maxAttempts,
			// InvoiceID and CustomerID are filled by CreateInvoiceTx
		},
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.CreateInvoiceResponse{
		Invoice:  dto.ToInvoiceResponse(result.Invoice),
		Reminder: dto.ToReminderResponse(result.Reminder),
	}, nil
}

// GetInvoice returns a single invoice owned by userID.
func (s *InvoiceService) GetInvoice(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
) (*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	inv, err := s.store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
		ID:     invoiceID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToInvoiceResponse(inv), nil
}

// ListInvoices returns a paginated list of invoices with optional filters.
// Collection-state filtering is applied in Go after the DB query because there
// is no dedicated SQL query for that filter combination.
func (s *InvoiceService) ListInvoices(
	ctx context.Context,
	userID uuid.UUID,
	filter *dto.ListInvoicesFilter,
	pg *dto.Pagination,
) ([]*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	var rows []db.Invoice
	var err error

	switch {
	case filter != nil && filter.CustomerID != nil:
		rows, err = s.store.ListInvoicesByCustomer(dbCtx, db.ListInvoicesByCustomerParams{
			UserID:     userID,
			CustomerID: *filter.CustomerID,
			Limit:      pg.Limit32(),
			Offset:     pg.Offset32(),
		})
	case filter != nil && filter.Overdue:
		rows, err = s.store.ListOverdueInvoices(dbCtx, db.ListOverdueInvoicesParams{
			UserID: userID,
			Limit:  pg.Limit32(),
			Offset: pg.Offset32(),
		})
	case filter != nil && filter.Status != nil:
		rows, err = s.store.ListInvoicesByStatus(dbCtx, db.ListInvoicesByStatusParams{
			UserID: userID,
			Status: *filter.Status,
			Limit:  pg.Limit32(),
			Offset: pg.Offset32(),
		})
	default:
		rows, err = s.store.ListInvoices(dbCtx, db.ListInvoicesParams{
			UserID: userID,
			Limit:  pg.Limit32(),
			Offset: pg.Offset32(),
		})
	}
	if err != nil {
		return nil, api.FromPgError(err)
	}

	// Apply collection_state filter in Go — no dedicated SQL query for this field.
	if filter != nil && filter.CollectionState != nil {
		target := *filter.CollectionState
		filtered := rows[:0]
		for _, r := range rows {
			if r.CollectionState == target {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	out := make([]*dto.InvoiceResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToInvoiceResponse(r))
	}
	return out, nil
}

// CancelInvoice cancels an invoice, kills its pending reminders, and reverses
// the unpaid balance from the customer's total_owed.
//
//nolint:dupl // mirrors WriteOffInvoice structure; both fetch-then-tx patterns differ only in the Tx called
func (s *InvoiceService) CancelInvoice(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
) (*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	// Fetch customer_id upfront — CancelInvoiceTx needs it for the debt reversal.
	inv, err := s.store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
		ID:     invoiceID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	result, err := s.store.CancelInvoiceTx(dbCtx, db.CancelInvoiceTxParams{
		InvoiceID:  invoiceID,
		UserID:     userID,
		CustomerID: inv.CustomerID,
	})
	if err != nil {
		if errors.Is(err, db.ErrInvoiceAlreadyCanceled) {
			return nil, api.ErrConflict("invoice is already canceled")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToInvoiceResponse(result.Invoice), nil
}

// WriteOffInvoice marks an invoice as defaulted and canceled, cancels its
// pending reminders, and emits an invoice_written_off event.
//
//nolint:dupl // mirrors CancelInvoice structure; both fetch-then-tx patterns differ only in the Tx called
func (s *InvoiceService) WriteOffInvoice(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
) (*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	inv, err := s.store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
		ID:     invoiceID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	result, err := s.store.WriteOffInvoiceTx(dbCtx, db.WriteOffInvoiceTxParams{
		InvoiceID:  invoiceID,
		UserID:     userID,
		CustomerID: inv.CustomerID,
	})
	if err != nil {
		if errors.Is(err, db.ErrInvoiceAlreadyCanceled) {
			return nil, api.ErrConflict("invoice is already written off")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToInvoiceResponse(result.Invoice), nil
}

// MoveCollectionState validates and applies a manual collection-state transition,
// emitting a collection_state_changed event for the History timeline.
func (s *InvoiceService) MoveCollectionState(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
	req *dto.MoveCollectionStateRequest,
) (*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	inv, err := s.store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
		ID:     invoiceID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	if !isValidTransition(inv.CollectionState, req.State) {
		return nil, api.ErrInvalidStateTransition(inv.CollectionState, req.State)
	}

	result, err := s.store.MoveCollectionStateTx(dbCtx, db.MoveCollectionStateTxParams{
		InvoiceID:       invoiceID,
		UserID:          userID,
		CustomerID:      inv.CustomerID,
		CollectionState: req.State,
		Note:            req.Note,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToInvoiceResponse(result.Invoice), nil
}
