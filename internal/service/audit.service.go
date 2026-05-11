package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"

	"github.com/google/uuid"
)

func (s *AuditService) ListAuditLogs(
	ctx context.Context,
	userID uuid.UUID,
	pg *dto.Pagination,
	actionType string,
) ([]*dto.AuditLogResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	var rows []db.OperatorAuditLog
	var err error

	if actionType != "" {
		rows, err = s.store.ListOperatorAuditLogsByActionType(dbCtx, db.ListOperatorAuditLogsByActionTypeParams{
			UserID:     userID,
			ActionType: db.AuditActionType(actionType),
			Limit:      pg.Limit32(),
			Offset:     pg.Offset32(),
		})
	} else {
		rows, err = s.store.ListOperatorAuditLogs(dbCtx, db.ListOperatorAuditLogsParams{
			UserID: userID,
			Limit:  pg.Limit32(),
			Offset: pg.Offset32(),
		})
	}
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.AuditLogResponse, 0, len(rows))
	for _, a := range rows {
		out = append(out, dto.ToAuditLogResponse(a))
	}
	return out, nil
}

func (s *AuditService) RevertAuditLog(
	ctx context.Context,
	logID, performedBy uuid.UUID,
) (*dto.AuditLogResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	result, err := s.store.MarkOperatorAuditLogReverted(dbCtx, db.MarkOperatorAuditLogRevertedParams{
		ID:         logID,
		RevertedBy: &performedBy,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrNotFound("audit log entry not found")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToAuditLogResponse(result), nil
}
