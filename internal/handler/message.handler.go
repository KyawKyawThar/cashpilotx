package handler

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

// MessageHandler handles AI-draft and manual message endpoints.
type MessageHandler struct {
	*BaseHandler
	svc service.MessageServicer
}

func NewMessageHandler(svc service.MessageServicer, base *BaseHandler) *MessageHandler {
	return &MessageHandler{BaseHandler: base, svc: svc}
}

// ListMessages godoc
//
//	@Summary      List messages for an invoice
//	@Description  Returns all messages (AI drafts and manual) for an invoice, newest first.
//	@Tags         messages
//	@Produce      json
//	@Param        invoice_id  path      string  true  "Invoice UUID"
//	@Success      200         {array}   dto.ChatMessageResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/invoices/{invoice_id}/messages [get]
//	@Security     BearerAuth
func (h *MessageHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	invoiceID, ok := h.MustUUIDParam(w, r, "invoice_id")
	if !ok {
		return
	}

	resp, err := h.svc.ListMessages(r.Context(), invoiceID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// CreateManualMessage godoc
//
//	@Summary      Create manual message
//	@Description  Creates a message written by the operator (source=human, status=approved), bypassing AI draft generation.
//	@Tags         messages
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateManualMessageRequest  true  "Message payload"
//	@Success      201   {object}  dto.ChatMessageResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/messages/manual [post]
//	@Security     BearerAuth
func (h *MessageHandler) CreateManualMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req dto.CreateManualMessageRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.CreateManualMessage(r.Context(), userID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, resp)
}

// ApproveMessage godoc
//
//	@Summary      Approve AI draft
//	@Description  Moves a draft message to pending status, signaling the operator has approved it for sending.
//	@Tags         messages
//	@Produce      json
//	@Param        message_id  path      string  true  "Message UUID"
//	@Success      200         {object}  dto.ChatMessageResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/messages/{message_id}/approve [post]
//	@Security     BearerAuth
func (h *MessageHandler) ApproveMessage(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	messageID, ok := h.MustUUIDParam(w, r, "message_id")
	if !ok {
		return
	}

	resp, err := h.svc.ApproveMessage(r.Context(), messageID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// MarkMessageSent godoc
//
//	@Summary      Mark message sent
//	@Description  Records that the operator manually sent the message via LINE/WhatsApp. Optionally stores the external message ID for delivery tracking.
//	@Tags         messages
//	@Accept       json
//	@Produce      json
//	@Param        message_id  path      string                      true   "Message UUID"
//	@Param        body        body      dto.MarkMessageSentRequest  false  "Optional external message ID"
//	@Success      200         {object}  dto.ChatMessageResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/messages/{message_id}/mark-sent [post]
//	@Security     BearerAuth
//
//nolint:dupl // same handler skeleton as ReminderHandler.UpdateReminder; both are intentionally separate
func (h *MessageHandler) MarkMessageSent(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	messageID, ok := h.MustUUIDParam(w, r, "message_id")
	if !ok {
		return
	}

	var req dto.MarkMessageSentRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.MarkMessageSent(r.Context(), messageID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// GetCopyText godoc
//
//	@Summary      Get message copy text
//	@Description  Returns just the content of an approved message — the text the operator copy-pastes into LINE/WhatsApp.
//	@Tags         messages
//	@Produce      json
//	@Param        message_id  path      string  true  "Message UUID"
//	@Success      200         {object}  dto.CopyTextResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      404         {object}  dto.ErrorResponse
//	@Failure      500         {object}  dto.ErrorResponse
//	@Router       /api/v1/messages/{message_id}/copy-text [get]
//	@Security     BearerAuth
func (h *MessageHandler) GetCopyText(w http.ResponseWriter, r *http.Request) {
	_, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	messageID, ok := h.MustUUIDParam(w, r, "message_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetCopyText(r.Context(), messageID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}
