package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"

	"github.com/google/uuid"
)

func (s *ReminderService) ListReminders(
	ctx context.Context,
	invoiceID uuid.UUID,
) ([]*dto.ReminderResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListRemindersByInvoice(dbCtx, invoiceID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.ReminderResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToReminderResponse(r))
	}
	return out, nil
}

func (s *ReminderService) CreateReminder(
	ctx context.Context,
	userID, invoiceID uuid.UUID,
	req *dto.CreateReminderRequest,
) (*dto.ReminderResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	// Enforce one active reminder per invoice.
	_, err := s.store.GetActiveReminderByInvoice(dbCtx, invoiceID)
	if err == nil {
		return nil, api.NewError(api.ErrCodeConflict, "an active reminder already exists for this invoice", 409)
	}
	if !api.IsRecordNotFound(err) {
		return nil, api.FromPgError(err)
	}

	tone := req.Tone
	if tone == "" {
		tone = defaultTone
	}
	channel := req.Channel
	if channel == "" {
		channel = "line"
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 5
	}

	reminder, err := s.store.CreateReminder(dbCtx, db.CreateReminderParams{
		UserID:      userID,
		InvoiceID:   invoiceID,
		CustomerID:  req.CustomerID,
		ScheduledAt: req.ScheduledAt,
		Tone:        tone,
		Channel:     channel,
		MaxAttempts: maxAttempts,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToReminderResponse(reminder), nil
}

//nolint:dupl // identical paginated-list pattern shared across services; abstraction adds no value
func (s *ReminderService) ListAllReminders(
	ctx context.Context,
	userID uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.ReminderResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListRemindersByUser(dbCtx, db.ListRemindersByUserParams{
		UserID: userID,
		Limit:  pg.Limit32(),
		Offset: pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.ReminderResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToReminderResponse(r))
	}
	return out, nil
}

func (s *ReminderService) CancelReminder(
	ctx context.Context,
	reminderID uuid.UUID,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.store.CancelReminder(dbCtx, reminderID); err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrNotFound("reminder not found")
		}
		return api.FromPgError(err)
	}
	return nil
}

func (s *ReminderService) MarkReminderSent(
	ctx context.Context,
	reminderID uuid.UUID,
	req *dto.MarkReminderSentRequest,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	err := s.store.MarkReminderSent(dbCtx, db.MarkReminderSentParams{
		ID:                reminderID,
		ExternalMessageID: req.ExternalMessageID,
	})
	if err != nil {
		return api.FromPgError(err)
	}
	return nil
}
