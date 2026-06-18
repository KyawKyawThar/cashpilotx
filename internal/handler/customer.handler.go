package handler

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// CustomerHandler handles customer CRUD and channel config endpoints.
type CustomerHandler struct {
	*BaseHandler
	svc service.CustomerServicer
}

func NewCustomerHandler(svc service.CustomerServicer, base *BaseHandler) *CustomerHandler {
	return &CustomerHandler{BaseHandler: base, svc: svc}
}

// mustChannelParam extracts the "channel" path parameter.
func (h *CustomerHandler) mustChannelParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	v := chi.URLParam(r, "channel")
	if v == "" {
		h.WriteErr(w, r, api.ErrBadRequest("missing channel path parameter"))
		return "", false
	}
	return v, true
}

// =============================================================================
// Customer CRUD
// =============================================================================

// CreateCustomer godoc
//
//	@Summary      Create customer
//	@Description  Create a new customer record for the authenticated user.
//	@Tags         customers
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateCustomerRequest  true  "Customer payload"
//	@Success      201   {object}  dto.CustomerResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      409   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/customers [post]
//	@Security     BearerAuth
func (h *CustomerHandler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	var req dto.CreateCustomerRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	// For scoped roles (sales), tag the customer as assigned to the creator
	// so ListCustomersAssigned can filter by assignedTo = callerID.
	var assignedTo *uuid.UUID
	if scope.Scope == scopeOwn {
		assignedTo = &scope.CallerID
	}

	resp, err := h.svc.CreateCustomer(r.Context(), effectiveUserID, assignedTo, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// GetCustomer godoc
//
//	@Summary      Get customer
//	@Description  Fetch a single customer by ID.
//	@Tags         customers
//	@Produce      json
//	@Param        customer_id  path      string  true  "Customer UUID"
//	@Success      200          {object}  dto.CustomerResponse
//	@Failure      400          {object}  dto.ErrorResponse
//	@Failure      401          {object}  dto.ErrorResponse
//	@Failure      404          {object}  dto.ErrorResponse
//	@Failure      500          {object}  dto.ErrorResponse
//	@Router       /api/v1/customers/{customer_id} [get]
//	@Security     BearerAuth
//
//nolint:dupl // handler shape mirrors ListMessages/ListReminders; different service call makes extraction pointless
func (h *CustomerHandler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	customerID, ok := h.MustUUIDParam(w, r, "customer_id")
	if !ok {
		return
	}

	var callerID *uuid.UUID
	if scope.Scope == scopeOwn {
		callerID = &scope.CallerID
	}

	resp, err := h.svc.GetCustomer(r.Context(), effectiveUserID, customerID, callerID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ListCustomers godoc
//
//	@Summary      List customers
//	@Description  Return a paginated list of active customers for the authenticated user.
//	@Tags         customers
//	@Produce      json
//	@Param        page      query     int  false  "Page number (default 1)"
//	@Param        per_page  query     int  false  "Items per page (default 25, max 100)"
//	@Success      200       {object}  dto.Response{data=[]dto.CustomerResponse}
//	@Failure      401       {object}  dto.ErrorResponse
//	@Failure      500       {object}  dto.ErrorResponse
//	@Router       /api/v1/customers [get]
//	@Security     BearerAuth
func (h *CustomerHandler) ListCustomers(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	pg := dto.GetPaginationFromRequest(r)

	var (
		customers []*dto.CustomerResponse
		err       error
	)
	invoiceFilter := r.URL.Query().Get("invoice_filter") // "has" | "none" | ""
	filter := &dto.ListCustomersFilter{InvoiceFilter: invoiceFilter}
	var assignedTo *uuid.UUID
	switch {
	case scope.Scope == scopeOwn:
		filter.AssignedTo = &scope.CallerID
		assignedTo = &scope.CallerID
		customers, err = h.svc.ListCustomers(r.Context(), effectiveUserID, filter, pg)
	case r.URL.Query().Get("all") == queryTrue:
		if invoiceFilter != "" {
			customers, err = h.svc.ListCustomers(r.Context(), effectiveUserID, filter, pg)
		} else {
			customers, err = h.svc.ListAllCustomers(r.Context(), effectiveUserID, pg)
		}
	default:
		customers, err = h.svc.ListCustomers(r.Context(), effectiveUserID, filter, pg)
	}
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	var total int64
	if invoiceFilter == "has" || invoiceFilter == "none" {
		hasCount, noCount, countErr := h.svc.CountCustomersByInvoiceFilter(r.Context(), effectiveUserID, assignedTo, invoiceFilter)
		if countErr == nil {
			if invoiceFilter == "has" {
				total = hasCount
			} else {
				total = noCount
			}
			// Embed sub-counts in extra meta fields via pg.Total
			// We send counts for both chips via response headers so frontend can read them
			w.Header().Set("X-Count-Has-Invoice", fmt.Sprintf("%d", hasCount))
			w.Header().Set("X-Count-No-Invoice", fmt.Sprintf("%d", noCount))
		} else {
			total = int64(len(customers))
		}
	} else {
		total, err = h.svc.CountCustomers(r.Context(), effectiveUserID, assignedTo)
		if err != nil {
			total = int64(len(customers))
		}
		hasCount, noCount, countErr := h.svc.CountCustomersByInvoiceFilter(r.Context(), effectiveUserID, assignedTo, "")
		if countErr == nil {
			w.Header().Set("X-Count-Has-Invoice", fmt.Sprintf("%d", hasCount))
			w.Header().Set("X-Count-No-Invoice", fmt.Sprintf("%d", noCount))
		}
	}
	pg.Total = total
	dto.WriteSuccessWithMeta(w, r, customers, pg)
}

// SearchCustomers godoc
//
//	@Summary      Search customers
//	@Description  Full-text search across customer name and phone (max 20 results).
//	@Tags         customers
//	@Produce      json
//	@Param        q    query     string  true  "Search query"
//	@Success      200  {object}  dto.Response{data=[]dto.CustomerResponse}
//	@Failure      400  {object}  dto.ErrorResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/customers/search [get]
//	@Security     BearerAuth
func (h *CustomerHandler) SearchCustomers(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		h.WriteErr(w, r, api.ErrBadRequest("query parameter 'q' is required"))
		return
	}

	var callerID *uuid.UUID
	if scope.Scope == scopeOwn {
		callerID = &scope.CallerID
	}

	customers, err := h.svc.SearchCustomers(r.Context(), effectiveUserID, query, callerID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, customers)
}

// UpdateCustomer godoc
//
//	@Summary      Update customer
//	@Description  Partially update a customer's name, phone, country, notes, or tags.
//	@Tags         customers
//	@Accept       json
//	@Produce      json
//	@Param        customer_id  path      string                     true  "Customer UUID"
//	@Param        body         body      dto.UpdateCustomerRequest  true  "Fields to update"
//	@Success      200          {object}  dto.CustomerResponse
//	@Failure      400          {object}  dto.ErrorResponse
//	@Failure      401          {object}  dto.ErrorResponse
//	@Failure      404          {object}  dto.ErrorResponse
//	@Failure      500          {object}  dto.ErrorResponse
//	@Router       /api/v1/customers/{customer_id} [patch]
//	@Security     BearerAuth
func (h *CustomerHandler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	customerID, ok := h.MustUUIDParam(w, r, "customer_id")
	if !ok {
		return
	}

	var req dto.UpdateCustomerRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	var callerID *uuid.UUID
	if scope.Scope == scopeOwn {
		callerID = &scope.CallerID
	}

	resp, err := h.svc.UpdateCustomer(r.Context(), effectiveUserID, customerID, callerID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ArchiveCustomer godoc
//
//	@Summary      Archive customer
//	@Description  Soft-delete a customer by setting their status to inactive.
//	@Tags         customers
//	@Produce      json
//	@Param        customer_id  path  string  true  "Customer UUID"
//	@Success      204  "No Content"
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/customers/{customer_id} [delete]
//	@Security     BearerAuth
//
//nolint:dupl // mirrors ReactivateCustomer; different service method, extraction would add indirection
func (h *CustomerHandler) ArchiveCustomer(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	customerID, ok := h.MustUUIDParam(w, r, "customer_id")
	if !ok {
		return
	}

	var callerID *uuid.UUID
	if scope.Scope == scopeOwn {
		callerID = &scope.CallerID
	}

	if err := h.svc.ArchiveCustomer(r.Context(), effectiveUserID, customerID, callerID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// ReactivateCustomer godoc
//
//	@Summary      Reactivate customer
//	@Description  Restores an inactive customer to active status.
//	@Tags         customers
//	@Produce      json
//	@Param        customer_id  path  string  true  "Customer UUID"
//	@Success      204  "No Content"
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/customers/{customer_id}/reactivate [post]
//	@Security     BearerAuth
//
//nolint:dupl // mirrors ArchiveCustomer; different service method, extraction would add indirection
func (h *CustomerHandler) ReactivateCustomer(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	customerID, ok := h.MustUUIDParam(w, r, "customer_id")
	if !ok {
		return
	}

	var callerID *uuid.UUID
	if scope.Scope == scopeOwn {
		callerID = &scope.CallerID
	}

	if err := h.svc.ReactivateCustomer(r.Context(), effectiveUserID, customerID, callerID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// =============================================================================
// Channel Configs
// =============================================================================

// UpsertChannelConfig godoc
//
//	@Summary      Upsert channel config
//	@Description  Create or update a messaging channel config (e.g. WhatsApp, Messenger). The config JSON is encrypted at rest with AES-256-GCM.
//	@Tags         channel-configs
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.UpsertChannelConfigRequest  true  "Channel config payload"
//	@Success      200   {object}  dto.ChannelConfigResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/channel-configs [post]
//	@Security     BearerAuth
func (h *CustomerHandler) UpsertChannelConfig(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	var req dto.UpsertChannelConfigRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.UpsertChannelConfig(r.Context(), effectiveUserID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ListChannelConfigs godoc
//
//	@Summary      List channel configs
//	@Description  Return all channel configs for the authenticated user (config payload omitted — use GET /{channel} to decrypt).
//	@Tags         channel-configs
//	@Produce      json
//	@Success      200  {object}  dto.Response{data=[]dto.ChannelConfigResponse}
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/channel-configs [get]
//	@Security     BearerAuth
func (h *CustomerHandler) ListChannelConfigs(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	configs, err := h.svc.ListChannelConfigs(r.Context(), effectiveUserID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, configs)
}

// GetChannelConfig godoc
//
//	@Summary      Get channel config
//	@Description  Fetch a single channel config by channel name with decrypted config payload.
//	@Tags         channel-configs
//	@Produce      json
//	@Param        channel  path      string  true  "Channel name (e.g. whatsapp, messenger, line)"
//	@Success      200      {object}  dto.ChannelConfigResponse
//	@Failure      401      {object}  dto.ErrorResponse
//	@Failure      404      {object}  dto.ErrorResponse
//	@Failure      500      {object}  dto.ErrorResponse
//	@Router       /api/v1/channel-configs/{channel} [get]
//	@Security     BearerAuth
func (h *CustomerHandler) GetChannelConfig(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	channel, ok := h.mustChannelParam(w, r)
	if !ok {
		return
	}

	cfg, err := h.svc.GetChannelConfig(r.Context(), effectiveUserID, channel)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, cfg)
}

// DeleteChannelConfig godoc
//
//	@Summary      Delete channel config
//	@Description  Soft-delete a channel config by channel name.
//	@Tags         channel-configs
//	@Produce      json
//	@Param        channel  path  string  true  "Channel name"
//	@Success      204  "No Content"
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/channel-configs/{channel} [delete]
//	@Security     BearerAuth
func (h *CustomerHandler) DeleteChannelConfig(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	channel, ok := h.mustChannelParam(w, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteChannelConfig(r.Context(), effectiveUserID, channel); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// SetChannelActive godoc
//
//	@Summary      Enable / disable channel
//	@Description  Toggle the is_active flag on a channel config.
//	@Tags         channel-configs
//	@Accept       json
//	@Produce      json
//	@Param        channel  path      string                       true  "Channel name"
//	@Param        body     body      dto.SetChannelActiveRequest  true  "Active flag"
//	@Success      204  "No Content"
//	@Failure      400  {object}  dto.ErrorResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      404  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/channel-configs/{channel}/active [patch]
//	@Security     BearerAuth
func (h *CustomerHandler) SetChannelActive(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	channel, ok := h.mustChannelParam(w, r)
	if !ok {
		return
	}

	var req dto.SetChannelActiveRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := h.svc.SetChannelActive(r.Context(), effectiveUserID, channel, *req.IsActive); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// ListCustomerEvents godoc
//
//	@Summary      List customer events
//	@Description  Returns a paginated timeline of all events for a specific customer (payments, state changes, messages sent, etc.).
//	@Tags         customers
//	@Produce      json
//	@Param        customer_id  path      string  true   "Customer UUID"
//	@Param        page         query     int     false  "Page number (default 1)"
//	@Param        per_page     query     int     false  "Items per page (default 25, max 100)"
//	@Success      200          {array}   dto.CustomerEventResponse
//	@Failure      401          {object}  dto.ErrorResponse
//	@Failure      404          {object}  dto.ErrorResponse
//	@Failure      500          {object}  dto.ErrorResponse
//	@Router       /api/v1/customers/{customer_id}/events [get]
//	@Security     BearerAuth
func (h *CustomerHandler) ListCustomerEvents(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	customerID, ok := h.MustUUIDParam(w, r, "customer_id")
	if !ok {
		return
	}

	var callerID *uuid.UUID
	if scope.Scope == scopeOwn {
		callerID = &scope.CallerID
	}

	pg := dto.GetPaginationFromRequest(r)

	resp, err := h.svc.ListCustomerEvents(r.Context(), effectiveUserID, customerID, callerID, pg)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}
