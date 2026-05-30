package handler

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
	"time"
)

type ReportHandler struct {
	*BaseHandler
	svc service.ReportServicer
}

func NewReportHandler(svc service.ReportServicer, base *BaseHandler) *ReportHandler {
	return &ReportHandler{BaseHandler: base, svc: svc}
}

// GetDailyReport godoc
//
//	@Summary      Daily report
//	@Description  Returns all 6 report sections for a date range. sales role sees own rows only; cs_team sees their own team only.
//	@Tags         reports
//	@Produce      json
//	@Param        from_date      query     string  true   "Start date YYYY-MM-DD"
//	@Param        to_date        query     string  true   "End date YYYY-MM-DD (inclusive)"
//	@Param        tracking_code  query     string  false  "Filter by tracking code"
//	@Param        product_name   query     string  false  "Filter by product name"
//	@Param        sale_name      query     string  false  "Filter by salesperson name"
//	@Success      200  {object}  dto.DailyReportResponse
//	@Failure      400  {object}  dto.ErrorResponse
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      403  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/reports/daily [get]
//	@Security     BearerAuth
func (h *ReportHandler) GetDailyReport(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	fromStr := q.Get("from_date")
	toStr := q.Get("to_date")
	if fromStr == "" || toStr == "" {
		h.WriteErr(w, r, api.ErrBadRequest("from_date and to_date are required (YYYY-MM-DD)"))
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("invalid from_date — use YYYY-MM-DD"))
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("invalid to_date — use YYYY-MM-DD"))
		return
	}
	if to.Before(from) {
		h.WriteErr(w, r, api.ErrBadRequest("to_date must not be before from_date"))
		return
	}
	to = to.AddDate(0, 0, 1)

	filter := &dto.ReportFilter{
		FromDate:     from,
		ToDate:       to,
		TrackingCode: q.Get("tracking_code"),
		ProductName:  q.Get("product_name"),
		SaleName:     q.Get("sale_name"),
	}

	// effectiveUserID == org owner's UUID for all roles (see ScopeContext middleware).
	resp, svcErr := h.svc.GetDailyReport(r.Context(), effectiveUserID, scope.CallerID, scope.CallerRole, scope.TeamID, filter)
	if svcErr != nil {
		h.HandleErr(w, r, svcErr)
		return
	}

	h.WriteSuccess(w, r, resp)
}
