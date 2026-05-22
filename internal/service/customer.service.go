package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/crypto"
	"cashpilotx/internal/dto"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// channelConfigKeyID is used when there is only one encryption key in rotation.
const channelConfigKeyID = "v1"

// normalizeName returns a lowercase, trimmed version of the name for searching.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// normalizePhone strips all characters except digits and a leading '+'.
func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range phone {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *CustomerService) CreateCustomer(
	ctx context.Context,
	userID uuid.UUID,
	assignedTo *uuid.UUID,
	req *dto.CreateCustomerRequest,
) (*dto.CustomerResponse, error) {
	normalizedName := normalizeName(req.Name)

	var normalizedPhone *string
	if req.Phone != nil && *req.Phone != "" {
		if np := normalizePhone(*req.Phone); np != "" {
			normalizedPhone = &np
		}
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	result, err := s.store.CreateCustomerTx(dbCtx, db.CreateCustomerTxParams{
		Customer: db.CreateCustomerParams{
			UserID:          userID,
			Name:            req.Name,
			NormalizedName:  normalizedName,
			Phone:           req.Phone,
			NormalizedPhone: normalizedPhone,
			Country:         req.Country,
			Type:            req.Type,
			Notes:           req.Notes,
			Tags:            tags,
			AssignedTo:      assignedTo,
		},
	})
	if err != nil {
		if api.IsUniqueViolation(err) {
			return nil, api.ErrDuplicateResource("Customer", "phone")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToCustomerResponse(result.Customer), nil
}

func (s *CustomerService) GetCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
) (*dto.CustomerResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	customer, err := s.store.GetCustomerByID(dbCtx, db.GetCustomerByIDParams{
		ID:     customerID,
		UserID: userID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrCustomerNotFound()
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToCustomerResponse(customer), nil
}

func (s *CustomerService) ListCustomers(
	ctx context.Context,
	userID uuid.UUID,
	filter *dto.ListCustomersFilter,
	pg *dto.Pagination,
) ([]*dto.CustomerResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	var rows []db.Customer
	var err error

	if filter != nil && filter.AssignedTo != nil {
		rows, err = s.store.ListCustomersAssigned(dbCtx, db.ListCustomersAssignedParams{
			UserID:     userID,
			AssignedTo: filter.AssignedTo,
			Limit:      pg.Limit32(),
			Offset:     pg.Offset32(),
		})
	} else {
		rows, err = s.store.ListCustomers(dbCtx, db.ListCustomersParams{
			UserID: userID,
			Limit:  pg.Limit32(),
			Offset: pg.Offset32(),
		})
	}
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.CustomerResponse, 0, len(rows))
	for _, c := range rows {
		out = append(out, dto.ToCustomerResponse(c))
	}
	return out, nil
}

//nolint:dupl // identical paginated-list pattern shared across services; abstraction adds no value
func (s *CustomerService) ListAllCustomers(
	ctx context.Context,
	userID uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.CustomerResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListAllCustomers(dbCtx, db.ListAllCustomersParams{
		UserID: userID,
		Limit:  pg.Limit32(),
		Offset: pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.CustomerResponse, 0, len(rows))
	for _, c := range rows {
		out = append(out, dto.ToCustomerResponse(c))
	}
	return out, nil
}

func (s *CustomerService) SearchCustomers(
	ctx context.Context,
	userID uuid.UUID,
	query string,
) ([]*dto.CustomerResponse, error) {
	if strings.TrimSpace(query) == "" {
		return []*dto.CustomerResponse{}, nil
	}

	q := strings.ToLower(strings.TrimSpace(query))

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.SearchCustomers(dbCtx, db.SearchCustomersParams{
		UserID:  userID,
		Column2: &q,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.CustomerResponse, 0, len(rows))
	for _, c := range rows {
		out = append(out, dto.ToCustomerResponse(c))
	}
	return out, nil
}

func (s *CustomerService) UpdateCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
	req *dto.UpdateCustomerRequest,
) (*dto.CustomerResponse, error) {
	var normalizedName *string
	if req.Name != nil {
		if n := normalizeName(*req.Name); n != "" {
			normalizedName = &n
		}
	}

	var normalizedPhone *string
	if req.Phone != nil && *req.Phone != "" {
		if np := normalizePhone(*req.Phone); np != "" {
			normalizedPhone = &np
		}
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	var tags []string
	if req.Tags != nil {
		tags = *req.Tags
	}

	customer, err := s.store.UpdateCustomer(dbCtx, db.UpdateCustomerParams{
		ID:              customerID,
		UserID:          userID,
		Name:            req.Name,
		NormalizedName:  normalizedName,
		Phone:           req.Phone,
		NormalizedPhone: normalizedPhone,
		Country:         req.Country,
		Notes:           req.Notes,
		Tags:            tags,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrCustomerNotFound()
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToCustomerResponse(customer), nil
}

// ArchiveCustomer soft-deletes a customer by setting their status to "inactive".
//
//nolint:dupl // mirrors ReactivateCustomer structure; extracting a helper adds indirection with no benefit
func (s *CustomerService) ArchiveCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
) error {
	// Verify ownership before mutating.
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.store.GetCustomerByID(dbCtx, db.GetCustomerByIDParams{
		ID:     customerID,
		UserID: userID,
	}); err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrCustomerNotFound()
		}
		return api.FromPgError(err)
	}

	if err := s.store.SetCustomerStatus(dbCtx, db.SetCustomerStatusParams{
		ID:     customerID,
		UserID: userID,
		Status: "inactive",
	}); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

// ReactivateCustomer restores an inactive customer to active status.
//
//nolint:dupl // mirrors ArchiveCustomer structure; extracting a helper adds indirection with no benefit
func (s *CustomerService) ReactivateCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.store.GetCustomerByID(dbCtx, db.GetCustomerByIDParams{
		ID:     customerID,
		UserID: userID,
	}); err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrCustomerNotFound()
		}
		return api.FromPgError(err)
	}

	if err := s.store.SetCustomerStatus(dbCtx, db.SetCustomerStatusParams{
		ID:     customerID,
		UserID: userID,
		Status: "active",
	}); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

// =============================================================================
// Channel Configs
// =============================================================================

func (s *CustomerService) UpsertChannelConfig(
	ctx context.Context,
	userID uuid.UUID,
	req *dto.UpsertChannelConfigRequest,
) (*dto.ChannelConfigResponse, error) {
	if s.encKey == nil {
		return nil, api.ErrInternal(fmt.Errorf("channel encryption key is not configured"))
	}

	plaintext, err := json.Marshal(req.Config)
	if err != nil {
		return nil, api.ErrBadRequest("config is not valid JSON")
	}

	ciphertext, nonce, tag, err := crypto.Seal(s.encKey, plaintext)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("encrypt channel config: %w", err))
	}

	status := "disconnected"
	if req.IsActive {
		status = "connected"
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	cfg, err := s.store.UpsertChannelConfig(dbCtx, db.UpsertChannelConfigParams{
		UserID:          userID,
		Channel:         strings.ToLower(req.Channel),
		ConfigEncrypted: ciphertext,
		ConfigNonce:     nonce,
		ConfigTag:       tag,
		KeyID:           channelConfigKeyID,
		ConfigVersion:   1,
		IsActive:        req.IsActive,
		Status:          status,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToChannelConfigResponse(cfg, req.Config), nil
}

func (s *CustomerService) ListChannelConfigs(
	ctx context.Context,
	userID uuid.UUID,
) ([]*dto.ChannelConfigResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListChannelConfigs(dbCtx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.ChannelConfigResponse, 0, len(rows))
	for _, c := range rows {
		out = append(out, dto.ToChannelConfigResponse(c, nil))
	}
	return out, nil
}

func (s *CustomerService) GetChannelConfig(
	ctx context.Context,
	userID uuid.UUID,
	channel string,
) (*dto.ChannelConfigResponse, error) {
	if s.encKey == nil {
		return nil, api.ErrInternal(fmt.Errorf("channel encryption key is not configured"))
	}

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	cfg, err := s.store.GetActiveChannelConfig(dbCtx, db.GetActiveChannelConfigParams{
		UserID:  userID,
		Channel: strings.ToLower(channel),
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrNotFound("ChannelConfig")
		}
		return nil, api.FromPgError(err)
	}

	plaintext, err := crypto.Open(s.encKey, cfg.ConfigEncrypted, cfg.ConfigNonce, cfg.ConfigTag)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("decrypt channel config: %w", err))
	}

	return dto.ToChannelConfigResponse(cfg, json.RawMessage(plaintext)), nil
}

func (s *CustomerService) DeleteChannelConfig(
	ctx context.Context,
	userID uuid.UUID,
	channel string,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	cfg, err := s.store.GetChannelConfig(dbCtx, db.GetChannelConfigParams{
		UserID:  userID,
		Channel: strings.ToLower(channel),
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrNotFound("ChannelConfig")
		}
		return api.FromPgError(err)
	}

	if err := s.store.DeleteChannelConfig(dbCtx, cfg.ID); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

func (s *CustomerService) SetChannelActive(
	ctx context.Context,
	userID uuid.UUID,
	channel string,
	active bool,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	cfg, err := s.store.GetChannelConfig(dbCtx, db.GetChannelConfigParams{
		UserID:  userID,
		Channel: strings.ToLower(channel),
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrNotFound("ChannelConfig")
		}
		return api.FromPgError(err)
	}

	if err := s.store.SetChannelActive(dbCtx, db.SetChannelActiveParams{
		ID:       cfg.ID,
		IsActive: active,
	}); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

func (s *CustomerService) ListCustomerEvents(
	ctx context.Context,
	userID, customerID uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.CustomerEventResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListCustomerEvents(dbCtx, db.ListCustomerEventsParams{
		UserID:     userID,
		CustomerID: customerID,
		Limit:      pg.Limit32(),
		Offset:     pg.Offset32(),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.CustomerEventResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, dto.ToCustomerEventResponse(e))
	}
	return out, nil
}
