package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"

	"github.com/google/uuid"
)

func (s *InvoiceService) GetDashboardStats(
	ctx context.Context,
	userID uuid.UUID,
) (*dto.DashboardResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	stats, err := s.store.GetRecoveryStats(dbCtx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	monthly, err := s.store.GetMonthlyStats(dbCtx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.DashboardResponse{
		TotalRecoverable:   stats.TotalRecoverable,
		WaitingForReply:    stats.WaitingForReply,
		OpenActions:        stats.OpenActions,
		RemindedCount:      stats.RemindedCount,
		CollectedThisMonth: monthly.TotalCollected,
		TotalMessagesSent:  monthly.TotalMessagesSent,
	}, nil
}

//nolint:dupl // identical paginated-list pattern shared across services; abstraction adds no value
func (s *InvoiceService) GetRecoveryQueue(
	ctx context.Context,
	userID uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.RecoveryQueueItemResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.GetRecoveryQueue(dbCtx, db.GetRecoveryQueueParams{
		UserID: userID,
		Limit:  pg.Limit32(),
		Offset: pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.RecoveryQueueItemResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToRecoveryQueueItemResponse(r))
	}
	return out, nil
}
