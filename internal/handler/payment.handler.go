package handler

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"

	"github.com/google/uuid"
)

// PaymentHandler handles payment recording and listing endpoints.
type PaymentHandler struct {
	*BaseHandler
	svc service.PaymentServicer
}

func NewPaymentHandler(svc service.PaymentServicer, base *BaseHandler) *PaymentHandler {
	return &PaymentHandler{BaseHandler: base, svc: svc}
}

// RecordPayment godoc
//
//	@Summary      Record a payment
//	@Description  Atomically inserts a payment, recomputes invoice status, and emits a payment_received event. Returns the new payment and the updated invoice snapshot.
//	@Tags         payments
//	@Accept       json
//	@Produce      json
//	@Param        Idempotency-Key  header    string                    false  "Client-generated idempotency key (UUID)"
//	@Param        body             body      dto.RecordPaymentRequest  true   "Payment payload"
//	@Success      201              {object}  dto.RecordPaymentResponse
//	@Failure      400              {object}  dto.ErrorResponse
//	@Failure      401              {object}  dto.ErrorResponse
//	@Failure      404              {object}  dto.ErrorResponse
//	@Failure      409              {object}  dto.ErrorResponse
//	@Failure      422              {object}  dto.ErrorResponse
//	@Failure      500              {object}  dto.ErrorResponse
//	@Router       /api/v1/payments [post]
//	@Security     BearerAuth
func (h *PaymentHandler) RecordPayment(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	var req dto.RecordPaymentRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.RecordPayment(r.Context(), effectiveUserID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// ListPayments godoc
//
//	@Summary      List payments
//	@Description  Returns a paginated list of all payments for the authenticated user.
//	@Tags         payments
//	@Produce      json
//	@Param        page   query     int  false  "Page number (default 1)"
//	@Param        limit  query     int  false  "Items per page (default 20, max 100)"
//	@Success      200    {array}   dto.PaymentListItem
//	@Failure      401    {object}  dto.ErrorResponse
//	@Failure      500    {object}  dto.ErrorResponse
//	@Router       /api/v1/payments [get]
//	@Security     BearerAuth
func (h *PaymentHandler) ListPayments(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	if r.URL.Query().Get("today") == queryTrue {
		resp, err := h.svc.ListPaymentsToday(r.Context(), effectiveUserID)
		if err != nil {
			h.HandleErr(w, r, err)
			return
		}
		h.WriteSuccess(w, r, resp)
		return
	}

	pg := dto.GetPaginationFromRequest(r)

	resp, err := h.svc.ListPayments(r.Context(), effectiveUserID, pg)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// GetPayment godoc
//
//	@Summary      Get payment
//	@Description  Returns a single payment by ID, scoped to the authenticated user.
//	@Tags         payments
//	@Produce      json
//	@Param        payment_id  path      string  true  "Payment UUID"
//	@Success      200         {object}  dto.PaymentResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/payments/{payment_id} [get]
//	@Security     BearerAuth
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	paymentID, ok := h.MustUUIDParam(w, r, "payment_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetPayment(r.Context(), effectiveUserID, paymentID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// VoidPayment godoc
//
//	@Summary      Void a payment
//	@Description  Marks a payment as voided, recomputes the invoice status, and adjusts customer running totals.
//	@Tags         payments
//	@Accept       json
//	@Produce      json
//	@Param        payment_id  path      string  true  "Payment UUID"
//	@Param        body        body      dto.VoidPaymentRequest  true  "customer_id required to update totals"
//	@Success      200         {object}  dto.PaymentResponse
//	@Failure      400         {object}  dto.ErrorResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/payments/{payment_id}/void [post]
//	@Security     BearerAuth
func (h *PaymentHandler) VoidPayment(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	paymentID, ok := h.MustUUIDParam(w, r, "payment_id")
	if !ok {
		return
	}

	var req struct {
		CustomerID uuid.UUID `json:"customer_id" validate:"required"`
	}
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.VoidPayment(r.Context(), effectiveUserID, paymentID, req.CustomerID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// ListPaymentsByInvoice godoc
//
//	@Summary      List payments for an invoice
//	@Description  Returns all payments recorded against a specific invoice, scoped to the authenticated user.
//	@Tags         payments
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {array}   dto.PaymentResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/payments [get]
//	@Security     BearerAuth
func (h *PaymentHandler) ListPaymentsByInvoice(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.ListPaymentsByInvoice(r.Context(), effectiveUserID, invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}
