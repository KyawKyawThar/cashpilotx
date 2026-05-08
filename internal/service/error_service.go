package service

import (
	db "cashpilotx/db/sqlc"
	"context"

	"github.com/google/uuid"
)

// ErrorService handles agent error ingestion and error group queries.
type ErrorService struct {
}

func NewErrorService(store db.Store) *ErrorService {
	return &ErrorService{}
}

func (s *ErrorService) IngestBatch() error {

	return nil
}

// ListErrorGroups returns a paginated list of error groups for an environment.
// Supports filtering by status, error_type, and free-text search.
func (s *ErrorService) ListErrorGroups() {
	// Verify env belongs to tenant.

}

// GetErrorGroup returns a single error group by its ID. Optionally fetches
// the raw traceback from S3 when include_trace is true.
func (s *ErrorService) GetErrorGroup(
	ctx context.Context,
	tenantID, envID, errorID uuid.UUID,
	includeTrace bool,
) {

}

// GetErrorGroupBySignature returns a single error group by its signature.
func (s *ErrorService) GetErrorGroupBySignature(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	signature string,
) {

}

// UpdateErrorGroupStatus transitions an error group to open, acknowledged, or resolved.
// Resolved groups record the resolving user ID and timestamp.
func (s *ErrorService) UpdateErrorGroupStatus(
	ctx context.Context,
	tenantID, envID, errorID, userID uuid.UUID,
	status string,
) {

}
