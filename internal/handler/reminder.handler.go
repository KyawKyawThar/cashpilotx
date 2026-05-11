package handler

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

// ReminderHandler handles reminder scheduling and status endpoints.
type ReminderHandler struct {
	*BaseHandler
	svc service.ReminderServicer
}

func NewReminderHandler(svc service.ReminderServicer, base *BaseHandler) *ReminderHandler {
	return &ReminderHandler{BaseHandler: base, svc: svc}
}

// ListReminders godoc
//
//	@Summary      List reminders for an invoice
//	@Description  Returns all reminders for a specific invoice, newest first.
//	@Tags         reminders
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {array}   dto.ReminderResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/reminders [get]
//	@Security     BearerAuth
func (h *ReminderHandler) ListReminders(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.ListReminders(r.Context(), invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// CreateReminder godoc
//
//	@Summary      Create reminder
//	@Description  Schedules a new reminder for an invoice. Only one active reminder (pending/processing) is allowed per invoice at a time.
//	@Tags         reminders
//	@Accept       json
//	@Produce      json
//	@Param        invoice_id  path      string                     true  "Invoice UUID"
//	@Param        body        body      dto.CreateReminderRequest  true  "Reminder payload"
//	@Success      201         {object}  dto.ReminderResponse
//	@Failure      400         {object}  dto.ErrorResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      409         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/reminders [post]
//	@Security     BearerAuth
//
//nolint:dupl // standard decode-validate-call handler pattern; identical structure is intentional
func (h *ReminderHandler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	var req dto.CreateReminderRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.CreateReminder(r.Context(), userID, invoiceID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// ListAllReminders godoc
//
//	@Summary      List all reminders (Scheduled view)
//	@Description  Returns a paginated list of all reminders for the authenticated user across all invoices, newest first.
//	@Tags         reminders
//	@Produce      json
//	@Param        page      query     int  false  "Page number (default 1)"
//	@Param        per_page  query     int  false  "Items per page (default 25, max 100)"
//	@Success      200       {array}   dto.ReminderResponse
//	@Failure      401       {object}  dto.ErrorResponse
//	@Failure      500       {object}  dto.ErrorResponse
//	@Router       /api/v1/reminders [get]
//	@Security     BearerAuth
func (h *ReminderHandler) ListAllReminders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	pg := dto.GetPaginationFromRequest(r)

	resp, err := h.svc.ListAllReminders(r.Context(), userID, pg)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// CancelReminder godoc
//
//	@Summary      Cancel a reminder
//	@Description  Cancels a pending or processing reminder. Has no effect on already-sent or failed reminders.
//	@Tags         reminders
//	@Produce      json
//	@Param        reminder_id  path  string  true  "Reminder UUID"
//	@Success      204          "No Content"
//	@Failure      401          {object}  dto.ErrorResponse
//	@Failure      404          {object}  dto.ErrorResponse
//	@Failure      500          {object}  dto.ErrorResponse
//	@Router       /api/v1/reminders/{reminder_id} [delete]
//	@Security     BearerAuth
func (h *ReminderHandler) CancelReminder(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	reminderID, ok := h.MustUUIDParam(w, r, "reminder_id")
	if !ok {
		return
	}

	if err := h.svc.CancelReminder(r.Context(), reminderID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// MarkReminderSent godoc
//
//	@Summary      Mark reminder sent
//	@Description  Records that the reminder was successfully sent (status → sent). Optionally stores the external message ID for tracking.
//	@Tags         reminders
//	@Accept       json
//	@Produce      json
//	@Param        reminder_id  path      string                       true   "Reminder UUID"
//	@Param        body         body      dto.MarkReminderSentRequest  false  "Optional external message ID"
//	@Success      204          "No Content"
//	@Failure      401          {object}  dto.ErrorResponse
//	@Failure      404          {object}  dto.ErrorResponse
//	@Failure      500          {object}  dto.ErrorResponse
//	@Router       /api/v1/reminders/{reminder_id}/mark-sent [post]
//	@Security     BearerAuth
func (h *ReminderHandler) MarkReminderSent(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	reminderID, ok := h.MustUUIDParam(w, r, "reminder_id")
	if !ok {
		return
	}

	var req dto.MarkReminderSentRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := h.svc.MarkReminderSent(r.Context(), reminderID, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}
