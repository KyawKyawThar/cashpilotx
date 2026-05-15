package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/dto"
	"context"
	"time"

	"github.com/google/uuid"
)

const syncPageLimit = int32(500)

type SyncService struct {
	store db.Store
}

func NewSyncService(store db.Store) *SyncService {
	return &SyncService{store: store}
}

func (s *SyncService) GetSyncData(ctx context.Context, userID uuid.UUID, since time.Time) (*dto.SyncResponse, error) {
	invoices, err := s.store.SyncInvoices(ctx, db.SyncInvoicesParams{
		UserID:       userID,
		UpdatedAfter: since,
		Limit:        syncPageLimit,
	})
	if err != nil {
		return nil, err
	}

	customers, err := s.store.SyncCustomers(ctx, db.SyncCustomersParams{
		UserID:       userID,
		UpdatedAfter: since,
		Limit:        syncPageLimit,
	})
	if err != nil {
		return nil, err
	}

	invDTOs := make([]*dto.InvoiceResponse, len(invoices))
	for i, inv := range invoices {
		invDTOs[i] = dto.ToInvoiceResponse(inv)
	}

	custDTOs := make([]*dto.CustomerResponse, len(customers))
	for i, c := range customers {
		custDTOs[i] = dto.ToCustomerResponse(c)
	}

	return &dto.SyncResponse{
		Invoices:   invDTOs,
		Customers:  custDTOs,
		ServerTime: time.Now().UTC(),
	}, nil
}
