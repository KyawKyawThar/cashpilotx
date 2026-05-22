package handler

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

// AuditHandler handles operator audit log endpoints.
type AuditHandler struct {
	*BaseHandler
	svc service.AuditServicer
}

func NewAuditHandler(svc service.AuditServicer, base *BaseHandler) *AuditHandler {
	return &AuditHandler{BaseHandler: base, svc: svc}
}

// ListAuditLogs godoc
//
//	@Summary      List audit logs
//	@Description  Returns a paginated list of operator audit log entries. Optionally filter by action_type.
//	@Tags         audit
//	@Produce      json
//	@Param        action_type  query     string  false  "Filter by action type (e.g. payment_recorded, message_marked_sent)"
//	@Param        page         query     int     false  "Page number (default 1)"
//	@Param        per_page     query     int     false  "Items per page (default 25, max 100)"
//	@Success      200          {array}   dto.AuditLogResponse
//	@Failure      401          {object}  dto.ErrorResponse
//	@Failure      500          {object}  dto.ErrorResponse
//	@Router       /api/v1/audit-log [get]
//	@Security     BearerAuth
func (h *AuditHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	pg := dto.GetPaginationFromRequest(r)
	actionType := r.URL.Query().Get("action_type")

	resp, err := h.svc.ListAuditLogs(r.Context(), effectiveUserID, pg, actionType)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// RevertAuditLog godoc
//
//	@Summary      Revert audit log entry
//	@Description  Marks an audit log entry as reverted, recording who reverted it and when.
//	@Tags         audit
//	@Produce      json
//	@Param        log_id  path      string  true  "Audit log UUID"
//	@Success      200     {object}  dto.AuditLogResponse
//	@Failure      401     {object}  dto.ErrorResponse
//	@Failure      404     {object}  dto.ErrorResponse
//	@Failure      500     {object}  dto.ErrorResponse
//	@Router       /api/v1/audit-log/{log_id}/revert [post]
//	@Security     BearerAuth
func (h *AuditHandler) RevertAuditLog(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	logID, ok := h.MustUUIDParam(w, r, "log_id")
	if !ok {
		return
	}

	resp, err := h.svc.RevertAuditLog(r.Context(), logID, userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}
