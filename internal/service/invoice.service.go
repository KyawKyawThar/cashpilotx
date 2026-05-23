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

// validTransitions maps each collection_state to the states it may advance to.
// Terminal states (paid, defaulted) are not present as keys — no further transitions.
var validTransitions = map[string][]string{
	"pending":     {"reminded", "paid", "defaulted"},
	"reminded":    {"no_response", "negotiating", "paid", "defaulted"},
	"no_response": {"reminded", "paid", "defaulted"},
	"negotiating": {"promised", "paid", "defaulted"},
	"promised":    {"verifying", "paid", "defaulted"},
	"verifying":   {"paid", "promised", "defaulted"}, // confirmed → paid; not arrived → back to promised
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

// resolveAssigneeName looks up the username for a single assignee UUID.
// Returns nil if the UUID is nil or the user cannot be found.
func (s *InvoiceService) resolveAssigneeName(ctx context.Context, id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	u, err := s.store.GetUserByID(ctx, *id)
	if err != nil {
		return nil
	}
	if u.Username != nil && *u.Username != "" {
		return u.Username
	}
	return &u.Name
}

// resolveAssigneeNames batch-resolves unique assignee UUIDs from a slice of invoices.
// Returns a map of UUID → display name (username preferred, falls back to name).
func (s *InvoiceService) resolveAssigneeNames(ctx context.Context, rows []db.Invoice) map[uuid.UUID]string {
	seen := make(map[uuid.UUID]struct{})
	for _, r := range rows {
		if r.AssignedTo != nil {
			seen[*r.AssignedTo] = struct{}{}
		}
	}
	result := make(map[uuid.UUID]string, len(seen))
	for id := range seen {
		u, err := s.store.GetUserByID(ctx, id)
		if err != nil {
			continue
		}
		if u.Username != nil && *u.Username != "" {
			result[id] = *u.Username
		} else {
			result[id] = u.Name
		}
	}
	return result
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
		channel = defaultChannel
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
			UserID:          userID,
			CustomerID:      req.CustomerID,
			Amount:          req.Amount,
			Currency:        req.Currency,
			Description:     req.Description,
			DueDate:         req.DueDate,
			Source:          req.Source,
			PhotoUrl:        req.PhotoURL,
			ExternalID:      &externalID,
			Column10:        items,
			AssignedTo:      req.AssignedTo,
			DeliveryService: req.DeliveryService,
			TrackingNumber:  req.TrackingNumber,
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
		Invoice:  dto.ToInvoiceResponse(result.Invoice, s.resolveAssigneeName(dbCtx, result.Invoice.AssignedTo)),
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

	return dto.ToInvoiceResponse(inv, s.resolveAssigneeName(dbCtx, inv.AssignedTo)), nil
}

// ListInvoices returns a paginated list of invoices with optional filters.
func (s *InvoiceService) ListInvoices(
	ctx context.Context,
	userID uuid.UUID,
	filter *dto.ListInvoicesFilter,
	pg *dto.Pagination,
) ([]*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.queryInvoices(dbCtx, userID, filter, pg)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	rows = applyCollectionStateFilter(rows, filter)

	nameMap := s.resolveAssigneeNames(dbCtx, rows)
	out := make([]*dto.InvoiceResponse, 0, len(rows))
	for _, r := range rows {
		var assigneeName *string
		if r.AssignedTo != nil {
			if name, ok := nameMap[*r.AssignedTo]; ok {
				assigneeName = &name
			}
		}
		out = append(out, dto.ToInvoiceResponse(r, assigneeName))
	}
	return out, nil
}

// queryInvoices selects the right DB query based on the active filter.
func (s *InvoiceService) queryInvoices(
	ctx context.Context,
	userID uuid.UUID,
	filter *dto.ListInvoicesFilter,
	pg *dto.Pagination,
) ([]db.Invoice, error) {
	if filter != nil {
		switch {
		case filter.CustomerID != nil:
			return s.store.ListInvoicesByCustomer(ctx, db.ListInvoicesByCustomerParams{
				UserID:     userID,
				CustomerID: *filter.CustomerID,
				Limit:      pg.Limit32(),
				Offset:     pg.Offset32(),
			})
		case filter.Overdue:
			return s.store.ListOverdueInvoices(ctx, db.ListOverdueInvoicesParams{
				UserID: userID,
				Limit:  pg.Limit32(),
				Offset: pg.Offset32(),
			})
		case filter.Status != nil:
			return s.store.ListInvoicesByStatus(ctx, db.ListInvoicesByStatusParams{
				UserID: userID,
				Status: *filter.Status,
				Limit:  pg.Limit32(),
				Offset: pg.Offset32(),
			})
		case filter.CurrentMonthOnly:
			return s.store.ListInvoicesCurrentMonth(ctx, db.ListInvoicesCurrentMonthParams{
				UserID: userID,
				Limit:  pg.Limit32(),
				Offset: pg.Offset32(),
			})
		}
	}
	return s.store.ListInvoices(ctx, db.ListInvoicesParams{
		UserID: userID,
		Limit:  pg.Limit32(),
		Offset: pg.Offset32(),
	})
}

// applyCollectionStateFilter filters invoices by collection_state in Go because
// there is no dedicated SQL query for that filter combination.
func applyCollectionStateFilter(rows []db.Invoice, filter *dto.ListInvoicesFilter) []db.Invoice {
	if filter == nil || filter.CollectionState == nil {
		return rows
	}
	target := *filter.CollectionState
	filtered := rows[:0]
	for _, r := range rows {
		if r.CollectionState == target {
			filtered = append(filtered, r)
		}
	}
	return filtered
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

	return dto.ToInvoiceResponse(result.Invoice, nil), nil
}

// ReactivateInvoice is the mirror of CancelInvoice — restores a canceled invoice
// to unpaid/partial status, resets collection_state to 'pending', and adds the
// unpaid balance back to the customer's total_owed.
//
//nolint:dupl // mirrors CancelInvoice structure
func (s *InvoiceService) ReactivateInvoice(
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

	result, err := s.store.ReactivateInvoiceTx(dbCtx, db.ReactivateInvoiceTxParams{
		InvoiceID:  invoiceID,
		UserID:     userID,
		CustomerID: inv.CustomerID,
	})
	if err != nil {
		if errors.Is(err, db.ErrInvoiceNotCanceled) {
			return nil, api.ErrConflict("invoice is not canceled")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToInvoiceResponse(result.Invoice, nil), nil
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

	return dto.ToInvoiceResponse(result.Invoice, nil), nil
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

	// Stamp promised_at when transitioning into the promised state.
	if req.State == "promised" {
		promisedAt := req.PromisedAt
		if promisedAt == nil {
			now := time.Now()
			promisedAt = &now
		}
		if updated, setErr := s.store.SetInvoicePromised(dbCtx, db.SetInvoicePromisedParams{
			ID:         invoiceID,
			UserID:     userID,
			PromisedAt: promisedAt,
		}); setErr == nil {
			result.Invoice = updated
		}
	}

	// Write audit log asynchronously.
	noteStr := ""
	if req.Note != nil && *req.Note != "" {
		noteStr = " Note: " + *req.Note
	}
	detail := fmt.Sprintf(
		"Invoice moved to %q state from %q.%s",
		req.State, inv.CollectionState, noteStr,
	)
	meta, err := json.Marshal(map[string]any{
		"from": inv.CollectionState,
		"to":   req.State,
		"note": req.Note,
	})
	if err != nil {
		meta = nil
	}
	invID := result.Invoice.ID
	custID := result.Invoice.CustomerID
	logAuditAsync(ctx, s.store, db.CreateOperatorAuditLogParams{
		UserID:        userID,
		PerformedBy:   userID,
		ActionType:    db.AuditActionTypeCollectionStateChanged,
		EntityType:    db.AuditEntityTypeInvoice,
		ActionLabel:   "Collection State → " + req.State,
		Detail:        detail,
		InvoiceID:     &invID,
		InvoiceNumber: result.Invoice.ExternalID,
		CustomerID:    &custID,
		Metadata:      meta,
	})

	return dto.ToInvoiceResponse(result.Invoice, nil), nil
}

// UpdateInvoiceTracking sets or clears the tracking number on an existing invoice.
func (s *InvoiceService) UpdateInvoiceTracking(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
	req *dto.UpdateInvoiceTrackingRequest,
) (*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	updated, err := s.store.UpdateInvoiceTracking(dbCtx, db.UpdateInvoiceTrackingParams{
		ID:             invoiceID,
		UserID:         userID,
		TrackingNumber: req.TrackingNumber,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToInvoiceResponse(updated, nil), nil
}
