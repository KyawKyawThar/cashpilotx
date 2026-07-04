package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
func (s *InvoiceService) resolveAssigneeNames(ctx context.Context, rows []db.Invoice) map[uuid.UUID]string {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if r.AssignedTo != nil {
			ids = append(ids, *r.AssignedTo)
		}
	}
	return s.resolveAssigneeNamesByIDs(ctx, ids)
}

// resolveAssigneeNamesByIDs looks up display names for a list of user UUIDs.
// Returns a map of UUID → display name (username preferred, falls back to name).
func (s *InvoiceService) resolveAssigneeNamesByIDs(ctx context.Context, ids []uuid.UUID) map[uuid.UUID]string {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		seen[id] = struct{}{}
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

// digitsOnly strips all non-digit characters from s.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizePhoneQuery converts a raw search string into a digits-only substring
// suitable for matching against normalized_phone (stored as "+959…").
// Leading "0" is stripped so "09xxx" matches "+959xxx".
func normalizePhoneQuery(q string) string {
	d := digitsOnly(q)
	if d != "" && d[0] == '0' {
		d = d[1:]
	}
	return d
}

// ListInvoices returns a paginated list of invoices with optional filters.
// When filter.Search is set the search is executed against the DB (JOIN customers);
// for all other filter combinations the existing per-query path is used, with
// customer name/phone batch-resolved and embedded in the response.
func (s *InvoiceService) ListInvoices(
	ctx context.Context,
	userID uuid.UUID,
	filter *dto.ListInvoicesFilter,
	pg *dto.Pagination,
) ([]*dto.InvoiceResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if filter != nil && filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		return s.listInvoicesBySearch(dbCtx, userID, *filter.Search, filter.AssignedTo, pg)
	}

	rows, err := s.queryInvoices(dbCtx, userID, filter, pg)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	rows = applyCollectionStateFilter(rows, filter)
	customerMap := s.resolveCustomerData(dbCtx, userID, rows)
	nameMap := s.resolveAssigneeNames(dbCtx, rows)

	out := make([]*dto.InvoiceResponse, 0, len(rows))
	for _, r := range rows {
		resp := dto.ToInvoiceResponse(r, resolveFromMap(nameMap, r.AssignedTo))
		if c, ok := customerMap[r.CustomerID]; ok {
			resp.CustomerName = &c.Name
			resp.CustomerPhone = c.Phone
			resp.CustomerNumber = c.CustomerNumber
		}
		out = append(out, resp)
	}
	return out, nil
}

// ListDuplicateInvoices finds active invoices whose customer shares a phone
// number with another customer that also has an active invoice — the
// signature of two sales reps independently creating a customer record (and
// invoice) for the same real person, since each only sees their own scoped
// customers. Results are grouped by phone so CS/admin can review each
// cluster and cancel the duplicate.
func (s *InvoiceService) ListDuplicateInvoices(ctx context.Context, userID uuid.UUID) ([]*dto.DuplicateInvoiceGroup, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListDuplicateInvoiceCandidates(dbCtx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if r.AssignedTo != nil {
			ids = append(ids, *r.AssignedTo)
		}
	}
	nameMap := s.resolveAssigneeNamesByIDs(dbCtx, ids)

	groups := make([]*dto.DuplicateInvoiceGroup, 0)
	var current *dto.DuplicateInvoiceGroup
	for _, r := range rows {
		phone := ""
		if r.CustomerNormalizedPhone != nil {
			phone = *r.CustomerNormalizedPhone
		}
		if current == nil || current.Phone != phone {
			current = &dto.DuplicateInvoiceGroup{Phone: phone}
			groups = append(groups, current)
		}
		current.Invoices = append(current.Invoices, dto.ToInvoiceResponseFromDuplicateCandidate(r, resolveFromMap(nameMap, r.AssignedTo)))
	}
	return groups, nil
}

// listInvoicesBySearch handles the search path: joins customers in SQL and
// batch-resolves assignee names before mapping to response DTOs.
// When assignedTo is non-nil (sales scope) only rows belonging to that user are returned.
func (s *InvoiceService) listInvoicesBySearch(
	ctx context.Context,
	userID uuid.UUID,
	rawSearch string,
	assignedTo *uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.InvoiceResponse, error) {
	q := strings.ToLower(strings.TrimSpace(rawSearch))
	rows, err := s.store.SearchInvoices(ctx, db.SearchInvoicesParams{
		UserID:  userID,
		Column2: &q,
		Column3: normalizePhoneQuery(q),
		Limit:   pg.Limit32(),
		Offset:  pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	nameMap := s.resolveAssigneeNamesFromSearch(ctx, rows)
	out := make([]*dto.InvoiceResponse, 0, len(rows))
	for _, r := range rows {
		if assignedTo != nil && (r.AssignedTo == nil || *r.AssignedTo != *assignedTo) {
			continue
		}
		out = append(out, dto.ToInvoiceResponseFromSearch(r, resolveFromMap(nameMap, r.AssignedTo)))
	}
	return out, nil
}

// resolveAssigneeNamesFromSearch batch-resolves assignee display names from search result rows.
func (s *InvoiceService) resolveAssigneeNamesFromSearch(ctx context.Context, rows []db.SearchInvoicesRow) map[uuid.UUID]string {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if r.AssignedTo != nil {
			ids = append(ids, *r.AssignedTo)
		}
	}
	return s.resolveAssigneeNamesByIDs(ctx, ids)
}

// resolveFromMap looks up a display name pointer from a UUID→name map.
// Returns nil if id is nil or not present in the map.
func resolveFromMap(m map[uuid.UUID]string, id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	if name, ok := m[*id]; ok {
		return &name
	}
	return nil
}

// resolveCustomerData batch-fetches customer name/phone for a page of invoices.
func (s *InvoiceService) resolveCustomerData(
	ctx context.Context,
	userID uuid.UUID,
	rows []db.Invoice,
) map[uuid.UUID]db.GetCustomersByIDsRow {
	seen := make(map[uuid.UUID]struct{}, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if _, ok := seen[r.CustomerID]; !ok {
			seen[r.CustomerID] = struct{}{}
			ids = append(ids, r.CustomerID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	customers, err := s.store.GetCustomersByIDs(ctx, db.GetCustomersByIDsParams{
		UserID:  userID,
		Column2: ids,
	})
	if err != nil {
		return nil
	}
	m := make(map[uuid.UUID]db.GetCustomersByIDsRow, len(customers))
	for _, c := range customers {
		m[c.ID] = c
	}
	return m
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
		case filter.AssignedTo != nil:
			return s.store.ListInvoicesByAssignee(ctx, db.ListInvoicesByAssigneeParams{
				UserID:     userID,
				AssignedTo: filter.AssignedTo,
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

// ReturnInvoice marks an invoice as returned (goods came back after delivery),
// cancels pending reminders, and reverses the unpaid balance from customer total_owed.
// If paid_amount > 0, the response still carries it so the caller can warn about a refund.
func (s *InvoiceService) ReturnInvoice(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
	req *dto.ReturnInvoiceRequest,
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
	if inv.Status == "returned" {
		return nil, api.ErrConflict("invoice is already returned")
	}
	if inv.Status == "paid" {
		return nil, api.ErrConflict("paid invoice cannot be returned")
	}
	if inv.Status == "canceled" {
		return nil, api.ErrConflict("canceled invoice cannot be returned")
	}

	txParams := db.ReturnInvoiceTxParams{
		InvoiceID:  invoiceID,
		UserID:     userID,
		CustomerID: inv.CustomerID,
	}
	if req != nil {
		txParams.ReturnReason = req.ReturnReason
		txParams.ReturnItems = req.ReturnItems
		txParams.AssignedTo = req.SoldBy
	}
	result, err := s.store.ReturnInvoiceTx(dbCtx, txParams)
	if err != nil {
		if errors.Is(err, db.ErrInvoiceAlreadyReturned) {
			return nil, api.ErrConflict("invoice is already returned")
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

// UpdateInvoice edits the content of an invoice (items, amount, currency, due date, etc.).
// Only allowed when paid_amount == 0 (no payments have been recorded).
func (s *InvoiceService) UpdateInvoice(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
	req *dto.UpdateInvoiceRequest,
) (*dto.InvoiceResponse, error) {
	items := req.Items
	if len(items) == 0 {
		items = []byte("[]")
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	// Fetch current invoice to check editability and compute the debt delta.
	existing, err := s.store.GetInvoiceByID(dbCtx, db.GetInvoiceByIDParams{
		ID:     invoiceID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	if existing.PaidAmount > 0 {
		return nil, api.ErrConflict("invoice with recorded payments cannot be edited")
	}

	updated, err := s.store.UpdateInvoiceContent(dbCtx, db.UpdateInvoiceContentParams{
		ID:              invoiceID,
		UserID:          userID,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Description:     req.Description,
		Column6:         items,
		DueDate:         req.DueDate,
		AssignedTo:      req.AssignedTo,
		DeliveryService: req.DeliveryService,
		TrackingNumber:  req.TrackingNumber,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrInvoiceNotFound()
		}
		return nil, api.FromPgError(err)
	}

	// Adjust customer total_owed by the amount delta.
	delta := req.Amount - existing.Amount
	if delta != 0 {
		if _, err := s.store.AddCustomerDebt(dbCtx, db.AddCustomerDebtParams{
			ID:        existing.CustomerID,
			UserID:    userID,
			TotalOwed: delta,
		}); err != nil {
			return nil, api.FromPgError(err)
		}
	}

	return dto.ToInvoiceResponse(updated, s.resolveAssigneeName(dbCtx, updated.AssignedTo)), nil
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
