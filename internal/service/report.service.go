package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const totalLabel = "Total"

// GetDailyReport runs all 6 report queries concurrently and returns the
// combined response. cs_team callers see only their own team's rows.
func (s *ReportService) GetDailyReport(
	ctx context.Context,
	orgOwnerID uuid.UUID,
	callerRole db.UserRole,
	teamID *uuid.UUID,
	filter *dto.ReportFilter,
) (*dto.DailyReportResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	var teamNames map[string]struct{}
	if callerRole == db.UserRoleCsTeam && teamID != nil {
		var err error
		teamNames, err = s.buildTeamAllowlist(dbCtx, orgOwnerID, *teamID)
		if err != nil {
			return nil, err
		}
	}

	orgPtr := &orgOwnerID
	fromPtr := &filter.FromDate
	toPtr := &filter.ToDate

	var (
		productRows     []db.GetProductListReportRow
		saleRows        []db.GetSaleReportRow
		recoverCashRows []db.GetRecoverCashReportRow
		invoiceRows     []db.GetInvoiceReportRow
		scheduleRows    []db.GetScheduleReportRow
		paymentRows     []db.GetPaymentReportRow
	)

	g, gCtx := errgroup.WithContext(dbCtx)

	g.Go(func() error {
		rows, err := s.store.GetProductListReport(gCtx, db.GetProductListReportParams{
			OrgOwnerID: orgOwnerID,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
			Column4:    filter.TrackingCode,
			Column5:    filter.ProductName,
		})
		if err != nil {
			return api.FromPgError(err)
		}
		productRows = rows
		return nil
	})

	g.Go(func() error {
		rows, err := s.store.GetSaleReport(gCtx, db.GetSaleReportParams{
			OrgOwnerID: orgPtr,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		if err != nil {
			return api.FromPgError(err)
		}
		saleRows = rows
		return nil
	})

	g.Go(func() error {
		rows, err := s.store.GetRecoverCashReport(gCtx, db.GetRecoverCashReportParams{
			OrgOwnerID: orgPtr,
			PaidOn:     filter.FromDate,
			PaidOn_2:   filter.ToDate,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		if err != nil {
			return api.FromPgError(err)
		}
		recoverCashRows = rows
		return nil
	})

	g.Go(func() error {
		rows, err := s.store.GetInvoiceReport(gCtx, db.GetInvoiceReportParams{
			OrgOwnerID: orgPtr,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		if err != nil {
			return api.FromPgError(err)
		}
		invoiceRows = rows
		return nil
	})

	g.Go(func() error {
		rows, err := s.store.GetScheduleReport(gCtx, db.GetScheduleReportParams{
			OrgOwnerID:    orgPtr,
			ScheduledAt:   filter.FromDate,
			ScheduledAt_2: filter.ToDate,
			Column4:       filter.TrackingCode,
			Column5:       filter.SaleName,
		})
		if err != nil {
			return api.FromPgError(err)
		}
		scheduleRows = rows
		return nil
	})

	g.Go(func() error {
		rows, err := s.store.GetPaymentReport(gCtx, db.GetPaymentReportParams{
			OrgOwnerID: orgPtr,
			PaidOn:     filter.FromDate,
			PaidOn_2:   filter.ToDate,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		if err != nil {
			return api.FromPgError(err)
		}
		paymentRows = rows
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	resp := &dto.DailyReportResponse{
		FromDate: filter.FromDate.Format("2006-01-02"),
		ToDate:   filter.ToDate.Format("2006-01-02"),
	}

	resp.ProductList, resp.Totals.ProductList = mapProductRows(productRows)
	resp.Sale, resp.Totals.Sale = mapSaleRows(saleRows, teamNames)
	resp.RecoverCash, resp.Totals.RecoverCash = mapRecoverCashRows(recoverCashRows, teamNames)
	resp.Invoice, resp.Totals.Invoice = mapInvoiceRows(invoiceRows, teamNames)
	resp.Schedule, resp.Totals.Schedule = mapScheduleRows(scheduleRows, teamNames)
	resp.Payment, resp.Totals.Payment = mapPaymentRows(paymentRows, teamNames)

	return resp, nil
}

// buildTeamAllowlist returns the set of salesperson names for the given team.
// Call only when callerRole == cs_team and teamID is non-nil.
func (s *ReportService) buildTeamAllowlist(
	ctx context.Context,
	orgOwnerID uuid.UUID,
	teamID uuid.UUID,
) (map[string]struct{}, error) {
	users, err := s.store.ListActiveOrgUsers(ctx, &orgOwnerID)
	if err != nil {
		return nil, api.FromPgError(err)
	}
	names := make(map[string]struct{}, len(users))
	for _, u := range users {
		if u.TeamID != nil && *u.TeamID == teamID {
			names[u.Name] = struct{}{}
		}
	}
	return names, nil
}

// =============================================================================
// Row mappers — convert SQLC rows → DTOs and compute section totals
// =============================================================================

func mapProductRows(rows []db.GetProductListReportRow) (out []dto.ProductListRow, total dto.ProductListRow) {
	out = make([]dto.ProductListRow, 0, len(rows))
	total.ProductName = totalLabel
	for _, r := range rows {
		out = append(out, dto.ProductListRow{
			ProductName: r.ProductName,
			TotalQty:    r.TotalQty,
			Revenue:     r.Revenue,
			TotalOrder:  r.TotalOrder,
			TotalValue:  r.TotalValue,
		})
		total.TotalQty += r.TotalQty
		total.Revenue += r.Revenue
		total.TotalOrder += r.TotalOrder
		total.TotalValue += r.TotalValue
	}
	return
}

func mapSaleRows(rows []db.GetSaleReportRow, allowed map[string]struct{}) (out []dto.SaleRow, total dto.SaleRow) {
	out = make([]dto.SaleRow, 0, len(rows))
	total.SaleName = totalLabel
	for _, r := range rows {
		if allowed != nil {
			if _, ok := allowed[r.SaleName]; !ok {
				continue
			}
		}
		out = append(out, dto.SaleRow{
			SaleName:       r.SaleName,
			TotalCustomer:  r.TotalCustomer,
			CashRevenue:    r.CashRevenue,
			InvoiceCount:   r.InvoiceCount,
			InvoiceRevenue: r.InvoiceRevenue,
			TotalOrder:     r.TotalOrder,
			TotalValue:     r.TotalValue,
		})
		total.TotalCustomer += r.TotalCustomer
		total.CashRevenue += r.CashRevenue
		total.InvoiceCount += r.InvoiceCount
		total.InvoiceRevenue += r.InvoiceRevenue
		total.TotalOrder += r.TotalOrder
		total.TotalValue += r.TotalValue
	}
	return
}

// saleReportRowData holds the common fields shared by GetRecoverCashReportRow,
// GetInvoiceReportRow, and GetPaymentReportRow — all generated types with identical structure.
type saleReportRowData struct {
	SaleName   string
	Amount     int64
	TotalOrder int64
	TotalValue int64
}

func mapSaleReportRowData(rows []saleReportRowData, allowed map[string]struct{}) (out []dto.SaleReportRow, total dto.SaleReportRow) {
	out = make([]dto.SaleReportRow, 0, len(rows))
	total.SaleName = totalLabel
	for _, r := range rows {
		if allowed != nil {
			if _, ok := allowed[r.SaleName]; !ok {
				continue
			}
		}
		out = append(out, dto.SaleReportRow{
			SaleName:   r.SaleName,
			Count:      r.TotalOrder,
			Amount:     r.Amount,
			TotalOrder: r.TotalOrder,
			TotalValue: r.TotalValue,
		})
		total.Count += r.TotalOrder
		total.Amount += r.Amount
		total.TotalOrder += r.TotalOrder
		total.TotalValue += r.TotalValue
	}
	return
}

func mapRecoverCashRows(rows []db.GetRecoverCashReportRow, allowed map[string]struct{}) (out []dto.SaleReportRow, total dto.SaleReportRow) {
	data := make([]saleReportRowData, len(rows))
	for i, r := range rows {
		data[i] = saleReportRowData{SaleName: r.SaleName, Amount: r.Amount, TotalOrder: r.TotalOrder, TotalValue: r.TotalValue}
	}
	return mapSaleReportRowData(data, allowed)
}

func mapInvoiceRows(rows []db.GetInvoiceReportRow, allowed map[string]struct{}) (out []dto.SaleReportRow, total dto.SaleReportRow) {
	out = make([]dto.SaleReportRow, 0, len(rows))
	total.SaleName = totalLabel
	for _, r := range rows {
		if allowed != nil {
			if _, ok := allowed[r.SaleName]; !ok {
				continue
			}
		}
		outstanding := r.Amount - r.PaidAmount
		out = append(out, dto.SaleReportRow{
			SaleName:          r.SaleName,
			Count:             r.TotalOrder,
			Amount:            r.Amount,
			CollectedAmount:   r.PaidAmount,
			OutstandingAmount: outstanding,
			TotalOrder:        r.TotalOrder,
			TotalValue:        r.TotalValue,
		})
		total.Count += r.TotalOrder
		total.Amount += r.Amount
		total.CollectedAmount += r.PaidAmount
		total.OutstandingAmount += outstanding
		total.TotalOrder += r.TotalOrder
		total.TotalValue += r.TotalValue
	}
	return
}

func mapPaymentRows(rows []db.GetPaymentReportRow, allowed map[string]struct{}) (out []dto.SaleReportRow, total dto.SaleReportRow) {
	data := make([]saleReportRowData, len(rows))
	for i, r := range rows {
		data[i] = saleReportRowData{SaleName: r.SaleName, Amount: r.Amount, TotalOrder: r.TotalOrder, TotalValue: r.TotalValue}
	}
	return mapSaleReportRowData(data, allowed)
}

func mapScheduleRows(rows []db.GetScheduleReportRow, allowed map[string]struct{}) (out []dto.SaleReportRow, total dto.SaleReportRow) {
	out = make([]dto.SaleReportRow, 0, len(rows))
	total.SaleName = totalLabel
	for _, r := range rows {
		if allowed != nil {
			if _, ok := allowed[r.SaleName]; !ok {
				continue
			}
		}
		out = append(out, dto.SaleReportRow{
			SaleName:   r.SaleName,
			Count:      r.Count,
			TotalOrder: r.TotalOrder,
			TotalValue: r.TotalValue,
		})
		total.Count += r.Count
		total.TotalOrder += r.TotalOrder
		total.TotalValue += r.TotalValue
	}
	return
}
