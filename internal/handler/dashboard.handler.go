package handler

import (
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

// DashboardHandler serves the stats strip and recovery queue.
type DashboardHandler struct {
	*BaseHandler
	svc service.InvoiceServicer
}

func NewDashboardHandler(svc service.InvoiceServicer, base *BaseHandler) *DashboardHandler {
	return &DashboardHandler{BaseHandler: base, svc: svc}
}

// GetDashboardStats godoc
//
//	@Summary      Dashboard stats
//	@Description  Returns the stats strip: total recoverable, waiting for reply, open actions, reminded count, collected this month, and messages sent this month.
//	@Tags         dashboard
//	@Produce      json
//	@Success      200  {object}  dto.DashboardResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/dashboard [get]
//	@Security     BearerAuth
func (h *DashboardHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.GetDashboardStats(r.Context(), userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}

// GetRecoveryQueue godoc
//
//	@Summary      Recovery queue
//	@Description  Returns open invoices ranked by AI confidence score, with customer name and phone pre-joined.
//	@Tags         dashboard
//	@Produce      json
//	@Param        page      query     int  false  "Page number (default 1)"
//	@Param        per_page  query     int  false  "Items per page (default 25, max 100)"
//	@Success      200       {array}   dto.RecoveryQueueItemResponse
//	@Failure      401       {object}  dto.ErrorResponse
//	@Failure      500       {object}  dto.ErrorResponse
//	@Router       /api/v1/recovery-queue [get]
//	@Security     BearerAuth
func (h *DashboardHandler) GetRecoveryQueue(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	pg := dto.GetPaginationFromRequest(r)

	resp, err := h.svc.GetRecoveryQueue(r.Context(), userID, pg)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, resp)
}
