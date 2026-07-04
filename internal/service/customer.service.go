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
	callerID *uuid.UUID,
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

	// sales role: only return if the customer is assigned to the caller
	if callerID != nil && (customer.AssignedTo == nil || *customer.AssignedTo != *callerID) {
		return nil, api.ErrCustomerNotFound()
	}

	resp := dto.ToCustomerResponse(customer)
	if err := s.attachAssignedToNames(dbCtx, []*dto.CustomerResponse{resp}); err != nil {
		return nil, api.FromPgError(err)
	}
	return resp, nil
}

// attachAssignedToNames batch-resolves each response's AssignedTo user ID to
// a display name in a single query, so the caller doesn't join per row.
func (s *CustomerService) attachAssignedToNames(ctx context.Context, out []*dto.CustomerResponse) error {
	seen := make(map[uuid.UUID]struct{})
	var ids []uuid.UUID
	for _, c := range out {
		if c.AssignedTo == nil {
			continue
		}
		if _, ok := seen[*c.AssignedTo]; ok {
			continue
		}
		seen[*c.AssignedTo] = struct{}{}
		ids = append(ids, *c.AssignedTo)
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := s.store.GetUserNamesByIDs(ctx, ids)
	if err != nil {
		return err
	}
	names := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	for _, c := range out {
		if c.AssignedTo == nil {
			continue
		}
		if name, ok := names[*c.AssignedTo]; ok {
			c.AssignedToName = &name
		}
	}
	return nil
}

func (s *CustomerService) CountCustomers(ctx context.Context, userID uuid.UUID, assignedTo *uuid.UUID) (int64, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()
	if assignedTo != nil {
		return s.store.CountCustomersAssigned(dbCtx, db.CountCustomersAssignedParams{
			UserID:     userID,
			AssignedTo: assignedTo,
		})
	}
	return s.store.CountCustomers(dbCtx, userID)
}

func (s *CustomerService) CountCustomersByInvoiceFilter(ctx context.Context, userID uuid.UUID, assignedTo *uuid.UUID, _ string) (hasCount, noCount int64, err error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()
	p := db.CountCustomersHasOpenInvoiceParams{UserID: userID, AssignedTo: assignedTo}
	hasCount, err = s.store.CountCustomersHasOpenInvoice(dbCtx, p)
	if err != nil {
		return 0, 0, err
	}
	noCount, err = s.store.CountCustomersNoOpenInvoice(dbCtx, db.CountCustomersNoOpenInvoiceParams{UserID: userID, AssignedTo: assignedTo})
	if err != nil {
		return 0, 0, err
	}
	return hasCount, noCount, nil
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

	switch {
	case filter != nil && filter.InvoiceFilter == "has":
		rows, err = s.store.ListCustomersHasOpenInvoice(dbCtx, db.ListCustomersHasOpenInvoiceParams{
			UserID: userID, AssignedTo: filter.AssignedTo,
			Limit: pg.Limit32(), Offset: pg.Offset32(),
		})
	case filter != nil && filter.InvoiceFilter == "none":
		rows, err = s.store.ListCustomersNoOpenInvoice(dbCtx, db.ListCustomersNoOpenInvoiceParams{
			UserID: userID, AssignedTo: filter.AssignedTo,
			Limit: pg.Limit32(), Offset: pg.Offset32(),
		})
	case filter != nil && filter.AssignedTo != nil:
		rows, err = s.store.ListCustomersAssigned(dbCtx, db.ListCustomersAssignedParams{
			UserID:     userID,
			AssignedTo: filter.AssignedTo,
			Limit:      pg.Limit32(),
			Offset:     pg.Offset32(),
		})
	default:
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
	if err := s.attachAssignedToNames(dbCtx, out); err != nil {
		return nil, api.FromPgError(err)
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
	if err := s.attachAssignedToNames(dbCtx, out); err != nil {
		return nil, api.FromPgError(err)
	}
	return out, nil
}

func (s *CustomerService) SearchCustomers(
	ctx context.Context,
	userID uuid.UUID,
	query string,
	callerID *uuid.UUID,
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
		// sales role: skip customers not assigned to the caller
		if callerID != nil && (c.AssignedTo == nil || *c.AssignedTo != *callerID) {
			continue
		}
		out = append(out, dto.ToCustomerResponse(c))
	}
	if err := s.attachAssignedToNames(dbCtx, out); err != nil {
		return nil, api.FromPgError(err)
	}
	return out, nil
}

// verifyCustomerOwner fetches the customer and, when callerID is non-nil,
// confirms it is assigned to that caller. Returns ErrCustomerNotFound on any failure.
func (s *CustomerService) verifyCustomerOwner(ctx context.Context, userID, customerID uuid.UUID, callerID *uuid.UUID) error {
	c, err := s.store.GetCustomerByID(ctx, db.GetCustomerByIDParams{ID: customerID, UserID: userID})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrCustomerNotFound()
		}
		return api.FromPgError(err)
	}
	if callerID != nil && (c.AssignedTo == nil || *c.AssignedTo != *callerID) {
		return api.ErrCustomerNotFound()
	}
	return nil
}

func (s *CustomerService) UpdateCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
	callerID *uuid.UUID,
	req *dto.UpdateCustomerRequest,
) (*dto.CustomerResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.verifyCustomerOwner(dbCtx, userID, customerID, callerID); err != nil {
		return nil, err
	}

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

	resp := dto.ToCustomerResponse(customer)
	if err := s.attachAssignedToNames(dbCtx, []*dto.CustomerResponse{resp}); err != nil {
		return nil, api.FromPgError(err)
	}
	return resp, nil
}

// AssignCustomer reassigns a customer's owner to another org member, or
// clears the assignment when assignedTo is nil. Unlike UpdateCustomer this
// is gated by the customer:edit permission at the route level, since it
// changes which sales rep can see/manage the customer under RBAC scoping.
func (s *CustomerService) AssignCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
	assignedTo *uuid.UUID,
) (*dto.CustomerResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if assignedTo != nil {
		target, err := s.store.GetUserByID(dbCtx, *assignedTo)
		if err != nil {
			if api.IsRecordNotFound(err) {
				return nil, api.ErrBadRequest("assigned_to must be an active member of your team")
			}
			return nil, api.FromPgError(err)
		}
		belongsToOrg := target.ID == userID || (target.OrgOwnerID != nil && *target.OrgOwnerID == userID)
		if !belongsToOrg {
			return nil, api.ErrBadRequest("assigned_to must be an active member of your team")
		}
	}

	customer, err := s.store.AssignCustomer(dbCtx, db.AssignCustomerParams{
		ID:         customerID,
		UserID:     userID,
		AssignedTo: assignedTo,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrCustomerNotFound()
		}
		return nil, api.FromPgError(err)
	}

	resp := dto.ToCustomerResponse(customer)
	if err := s.attachAssignedToNames(dbCtx, []*dto.CustomerResponse{resp}); err != nil {
		return nil, api.FromPgError(err)
	}
	return resp, nil
}

// ArchiveCustomer soft-deletes a customer by setting their status to "inactive".
func (s *CustomerService) ArchiveCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
	callerID *uuid.UUID,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.verifyCustomerOwner(dbCtx, userID, customerID, callerID); err != nil {
		return err
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
func (s *CustomerService) ReactivateCustomer(
	ctx context.Context,
	userID, customerID uuid.UUID,
	callerID *uuid.UUID,
) error {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.verifyCustomerOwner(dbCtx, userID, customerID, callerID); err != nil {
		return err
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
	callerID *uuid.UUID,
	pg *dto.Pagination,
) ([]*dto.CustomerEventResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	if err := s.verifyCustomerOwner(dbCtx, userID, customerID, callerID); err != nil {
		return nil, err
	}

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
