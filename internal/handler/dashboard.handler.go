package handler

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
	"time"
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

// GetCollectedSummary godoc
//
//	@Summary      Collected summary for a date range
//	@Description  Returns total collected amount and payment count for a given date range. from/to are YYYY-MM-DD (to is inclusive).
//	@Tags         dashboard
//	@Produce      json
//	@Param        from  query     string  true  "Start date YYYY-MM-DD"
//	@Param        to    query     string  true  "End date YYYY-MM-DD (inclusive)"
//	@Success      200   {object}  dto.CollectedSummaryResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      500   {object}  dto.ErrorResponse
//	@Router       /api/v1/payments/summary [get]
//	@Security     BearerAuth
func (h *DashboardHandler) GetCollectedSummary(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		h.WriteErr(w, r, api.ErrBadRequest("from and to query params are required (YYYY-MM-DD)"))
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("invalid from date — use YYYY-MM-DD"))
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("invalid to date — use YYYY-MM-DD"))
		return
	}

	// Add 1 day so the selected to-date is inclusive in the SQL < comparison
	toExclusive := to.AddDate(0, 0, 1)

	resp, err := h.svc.GetCollectedSummary(r.Context(), userID, from, toExclusive)
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
