package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"strings"
	"time"

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

	collected, err := s.store.SumCollectedThisMonth(dbCtx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	var collectionRate int32
	if stats.TotalInvoiced > 0 {
		collectionRate = int32(stats.TotalCollected * 100 / stats.TotalInvoiced) //nolint:gosec // collection rate is a percentage (0–100), cannot overflow int32
	}

	return &dto.DashboardResponse{
		TotalRecoverable:   stats.TotalRecoverable,
		WaitingForReply:    stats.WaitingForReply,
		OpenActions:        stats.OpenActions,
		RemindedCount:      stats.RemindedCount,
		CollectedThisMonth: collected,
		TotalMessagesSent:  monthly.TotalMessagesSent,
		OpenInvoicesCount:  stats.OpenInvoicesCount,
		CollectionRate:     collectionRate,
	}, nil
}

func (s *InvoiceService) GetCollectedSummary(
	ctx context.Context,
	userID uuid.UUID,
	from, to time.Time,
) (*dto.CollectedSummaryResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	row, err := s.store.SumCollectedBetween(dbCtx, db.SumCollectedBetweenParams{
		UserID:  userID,
		Column2: from,
		Column3: to,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.CollectedSummaryResponse{
		Collected:    row.Collected,
		PaymentCount: row.PaymentCount,
		From:         from.Format("2006-01-02"),
		To:           to.AddDate(0, 0, -1).Format("2006-01-02"), // display inclusive end
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

// SearchRecoveryQueue returns open invoices matching the search query (tracking number,
// external_id, or customer name), ranked by confidence score.
func (s *InvoiceService) SearchRecoveryQueue(
	ctx context.Context,
	userID uuid.UUID,
	query string,
	pg *dto.Pagination,
) ([]*dto.RecoveryQueueItemResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	q := strings.ToLower(strings.TrimSpace(query))
	rows, err := s.store.SearchRecoveryQueue(dbCtx, db.SearchRecoveryQueueParams{
		UserID:  userID,
		Column2: &q,
		Limit:   pg.Limit32(),
		Offset:  pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.RecoveryQueueItemResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ToRecoveryQueueItemResponseFromSearch(r))
	}
	return out, nil
}
