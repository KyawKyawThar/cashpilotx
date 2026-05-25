package handler

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

// InvoiceHandler handles invoice CRUD and collection-state transition endpoints.
type InvoiceHandler struct {
	*BaseHandler
	svc service.InvoiceServicer
}

func NewInvoiceHandler(svc service.InvoiceServicer, base *BaseHandler) *InvoiceHandler {
	return &InvoiceHandler{BaseHandler: base, svc: svc}
}

// CreateInvoice godoc
//
//	@Summary      Create invoice
//	@Description  Atomically creates a new invoice and its initial reminder, generating the external_id (INV-YYYY-NNN) via the user's atomic invoice_sequence counter.
//	@Tags         invoices
//	@Accept       json
//	@Produce      json
//	@Param        Idempotency-Key  header    string                     false  "Client-generated idempotency key (UUID)"
//	@Param        body             body      dto.CreateInvoiceRequest   true   "Invoice payload"
//	@Success      201              {object}  dto.CreateInvoiceResponse
//	@Failure      400              {object}  dto.ErrorResponse
//	@Failure      401              {object}  dto.ErrorResponse
//	@Failure      404              {object}  dto.ErrorResponse
//	@Failure      500              {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices [post]
//	@Security     BearerAuth
func (h *InvoiceHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	var req dto.CreateInvoiceRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.CreateInvoice(r.Context(), effectiveUserID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// GetInvoice godoc
//
//	@Summary      Get invoice
//	@Description  Returns a single invoice by ID, scoped to the authenticated user.
//	@Tags         invoices
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {object}  dto.InvoiceResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id} [get]
//	@Security     BearerAuth
func (h *InvoiceHandler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetInvoice(r.Context(), effectiveUserID, invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ListInvoices godoc
//
//	@Summary      List invoices
//	@Description  Returns a paginated list of invoices with optional filters.
//	@Tags         invoices
//	@Produce      json
//	@Param        status            query     string  false  "Filter by status: unpaid|partial|paid|canceled"
//	@Param        overdue           query     bool    false  "Only overdue invoices"
//	@Param        collection_state  query     string  false  "Filter by collection state"
//	@Param        customer_id       query     string  false  "Filter by customer UUID"
//	@Param        page              query     int     false  "Page number (default 1)"
//	@Param        per_page          query     int     false  "Items per page (default 25, max 100)"
//	@Success      200               {array}   dto.InvoiceResponse
//	@Failure      401               {object}  dto.ErrorResponse
//	@Failure      500               {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices [get]
//	@Security     BearerAuth
func (h *InvoiceHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	pg := dto.GetPaginationFromRequest(r)
	filter := invoiceFilterFromQuery(r)

	// sales: enforce current-month-only restriction.
	if scope.Scope == scopeOwn {
		// Reject explicit history queries older than the current month.
		if r.URL.Query().Get("before") != "" {
			h.WriteErr(w, r, api.ErrForbidden("Invoice history beyond current month is not available for your role"))
			return
		}
		filter.CurrentMonthOnly = true
	}

	resp, err := h.svc.ListInvoices(r.Context(), effectiveUserID, filter, pg)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// CancelInvoice godoc
//
//	@Summary      Cancel invoice
//	@Description  Cancels an invoice, kills its pending reminders, and reverses the unpaid balance from the customer's total_owed.
//	@Tags         invoices
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {object}  dto.InvoiceResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      409         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/cancel [post]
//	@Security     BearerAuth
func (h *InvoiceHandler) CancelInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.CancelInvoice(r.Context(), effectiveUserID, invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ReturnInvoice godoc
//
//	@Summary      Return invoice
//	@Description  Marks an invoice as returned (goods came back after delivery), cancels pending reminders, and reverses the unpaid balance from the customer's total_owed. If paid_amount > 0, the caller should prompt a refund.
//	@Tags         invoices
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {object}  dto.InvoiceResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      409         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/return [post]
//	@Security     BearerAuth
func (h *InvoiceHandler) ReturnInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	var req dto.ReturnInvoiceRequest
	if r.ContentLength > 0 {
		if !h.DecodeAndValidate(w, r, &req) {
			return
		}
	}

	resp, err := h.svc.ReturnInvoice(r.Context(), effectiveUserID, invoiceID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ReactivateInvoice godoc
//
//	@Summary      Reactivate invoice
//	@Description  Restores a canceled invoice to unpaid/partial status, resets collection_state to pending, and adds the unpaid balance back to the customer's total_owed.
//	@Tags         invoices
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {object}  dto.InvoiceResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      409         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/reactivate [post]
//	@Security     BearerAuth
func (h *InvoiceHandler) ReactivateInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.ReactivateInvoice(r.Context(), effectiveUserID, invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// WriteOffInvoice godoc
//
//	@Summary      Write off invoice
//	@Description  Marks an invoice as defaulted and canceled, cancels its pending reminders, and emits an invoice_written_off event.
//	@Tags         invoices
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {object}  dto.InvoiceResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/write-off [post]
//	@Security     BearerAuth
func (h *InvoiceHandler) WriteOffInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.WriteOffInvoice(r.Context(), effectiveUserID, invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// MoveCollectionState godoc
//
//	@Summary      Move collection state
//	@Description  Validates and applies a manual collection-state transition, emitting a collection_state_changed event for the History timeline.
//	@Tags         invoices
//	@Accept       json
//	@Produce      json
//	@Param        Idempotency-Key  header    string                          false  "Client-generated idempotency key (UUID)"
//	@Param        invoice_id       path      string                          true   "Invoice UUID"
//	@Param        body             body      dto.MoveCollectionStateRequest  true   "State transition payload"
//	@Success      200              {object}  dto.InvoiceResponse
//	@Failure      400              {object}  dto.ErrorResponse
//	@Failure      401              {object}  dto.ErrorResponse
//	@Failure      404              {object}  dto.ErrorResponse
//	@Failure      422              {object}  dto.ErrorResponse
//	@Failure      500              {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/collection-state [patch]
//	@Security     BearerAuth
//
//nolint:dupl // standard decode-validate-call handler pattern; identical structure is intentional
func (h *InvoiceHandler) MoveCollectionState(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	var req dto.MoveCollectionStateRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.MoveCollectionState(r.Context(), effectiveUserID, invoiceID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

func (h *InvoiceHandler) PatchInvoiceTracking(w http.ResponseWriter, r *http.Request) { //nolint:dupl // standard decode-validate-call handler pattern; identical structure is intentional
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	var req dto.UpdateInvoiceTrackingRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.UpdateInvoiceTracking(r.Context(), effectiveUserID, invoiceID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// =============================================================================
// Query parsing helpers
// =============================================================================

func invoiceFilterFromQuery(r *http.Request) *dto.ListInvoicesFilter {
	q := r.URL.Query()
	filter := &dto.ListInvoicesFilter{}

	if v := q.Get("status"); v != "" {
		filter.Status = &v
	}
	if q.Get("overdue") == queryTrue {
		filter.Overdue = true
	}
	if v := q.Get("collection_state"); v != "" {
		filter.CollectionState = &v
	}
	if v := q.Get("customer_id"); v != "" {
		if id, err := parseUUID(v); err == nil {
			filter.CustomerID = &id
		}
	}
	if v := q.Get("assigned_to"); v != "" {
		if id, err := parseUUID(v); err == nil {
			filter.AssignedTo = &id
		}
	}

	return filter
}
