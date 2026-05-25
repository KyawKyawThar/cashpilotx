package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"math"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const totalLabel = "Total"

// GetDailyReport runs all 9 report queries concurrently and returns the
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

	data, err := s.fetchDailyReportData(dbCtx, orgOwnerID, filter)
	if err != nil {
		return nil, err
	}

	resp := &dto.DailyReportResponse{
		FromDate: filter.FromDate.Format("2006-01-02"),
		ToDate:   filter.ToDate.Format("2006-01-02"),
	}

	resp.ProductList, resp.Totals.ProductList = mapProductRows(data.productRows)
	resp.Sale, resp.Totals.Sale = mapSaleRows(data.saleRows, teamNames)
	resp.RecoverCash, resp.Totals.RecoverCash = mapRecoverCashRows(data.recoverCashRows, teamNames)
	resp.Invoice, resp.Totals.Invoice = mapInvoiceRows(data.invoiceRows, teamNames)
	resp.Schedule, resp.Totals.Schedule = mapScheduleRows(data.scheduleRows, teamNames)
	resp.Payment, resp.Totals.Payment = mapPaymentRows(data.paymentRows, teamNames)
	resp.TopOverdueCustomers = mapOverdueCustomers(data.overdueRows, teamNames)
	daysInPeriod := filter.ToDate.Sub(filter.FromDate).Hours() / 24
	resp.Summary = buildSummary(resp.Totals.Invoice, data.agingRow, data.collectionStats, daysInPeriod)

	return resp, nil
}

type dailyReportData struct {
	productRows     []db.GetProductListReportRow
	saleRows        []db.GetSaleReportRow
	recoverCashRows []db.GetRecoverCashReportRow
	invoiceRows     []db.GetInvoiceReportRow
	scheduleRows    []db.GetScheduleReportRow
	paymentRows     []db.GetPaymentReportRow
	agingRow        db.GetAgingReportRow
	collectionStats db.GetCollectionStatsRow
	overdueRows     []db.GetTopOverdueCustomersRow
}

func (s *ReportService) fetchDailyReportData(
	ctx context.Context,
	orgOwnerID uuid.UUID,
	filter *dto.ReportFilter,
) (*dailyReportData, error) {
	orgPtr := &orgOwnerID
	fromPtr := &filter.FromDate
	toPtr := &filter.ToDate
	data := &dailyReportData{}

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() (err error) {
		data.productRows, err = s.store.GetProductListReport(gCtx, db.GetProductListReportParams{
			OrgOwnerID: orgOwnerID,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
			Column4:    filter.TrackingCode,
			Column5:    filter.ProductName,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.saleRows, err = s.store.GetSaleReport(gCtx, db.GetSaleReportParams{
			OrgOwnerID:  orgPtr,
			CreatedAt:   filter.FromDate,
			CreatedAt_2: filter.ToDate,
			Column4:     filter.TrackingCode,
			Column5:     filter.SaleName,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.recoverCashRows, err = s.store.GetRecoverCashReport(gCtx, db.GetRecoverCashReportParams{
			OrgOwnerID: orgPtr,
			PaidOn:     filter.FromDate,
			PaidOn_2:   filter.ToDate,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.invoiceRows, err = s.store.GetInvoiceReport(gCtx, db.GetInvoiceReportParams{
			OrgOwnerID: orgPtr,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.scheduleRows, err = s.store.GetScheduleReport(gCtx, db.GetScheduleReportParams{
			OrgOwnerID:    orgPtr,
			ScheduledAt:   filter.FromDate,
			ScheduledAt_2: filter.ToDate,
			Column4:       filter.TrackingCode,
			Column5:       filter.SaleName,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.paymentRows, err = s.store.GetPaymentReport(gCtx, db.GetPaymentReportParams{
			OrgOwnerID: orgPtr,
			PaidOn:     filter.FromDate,
			PaidOn_2:   filter.ToDate,
			Column4:    filter.TrackingCode,
			Column5:    filter.SaleName,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.agingRow, err = s.store.GetAgingReport(gCtx, db.GetAgingReportParams{
			UserID:     orgOwnerID,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.collectionStats, err = s.store.GetCollectionStats(gCtx, db.GetCollectionStatsParams{
			UserID:     orgOwnerID,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
		})
		return api.FromPgError(err)
	})

	g.Go(func() (err error) {
		data.overdueRows, err = s.store.GetTopOverdueCustomers(gCtx, db.GetTopOverdueCustomersParams{
			UserID:     orgOwnerID,
			IssuedAt:   fromPtr,
			IssuedAt_2: toPtr,
		})
		return api.FromPgError(err)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return data, nil
}

func mapOverdueCustomers(rows []db.GetTopOverdueCustomersRow, allowed map[string]struct{}) []dto.OverdueCustomerRow {
	out := make([]dto.OverdueCustomerRow, 0, len(rows))
	for _, r := range rows {
		if allowed != nil {
			if _, ok := allowed[r.SaleName]; !ok {
				continue
			}
		}
		out = append(out, dto.OverdueCustomerRow{
			CustomerName:      r.CustomerName,
			Phone:             r.Phone,
			SaleName:          r.SaleName,
			InvoiceCount:      r.InvoiceCount,
			OutstandingAmount: r.OutstandingAmount,
			MaxDaysOverdue:    r.MaxDaysOverdue,
		})
	}
	return out
}

func buildSummary(inv dto.SaleReportRow, aging db.GetAgingReportRow, stats db.GetCollectionStatsRow, daysInPeriod float64) dto.ReportSummary {
	collRate := 0.0
	if inv.Amount > 0 {
		collRate = math.Round(float64(inv.CollectedAmount)/float64(inv.Amount)*1000) / 10
	}
	dso := 0.0
	if inv.Amount > 0 {
		dso = math.Round(float64(inv.OutstandingAmount)/float64(inv.Amount)*daysInPeriod*10) / 10
	}
	p2p := 0.0
	if stats.TotalInCollection > 0 {
		p2p = math.Round(float64(stats.CollectionPaid)/float64(stats.TotalInCollection)*1000) / 10
	}
	return dto.ReportSummary{
		TotalInvoiced:     inv.Amount,
		TotalOutstanding:  inv.OutstandingAmount,
		CollectionRate:    collRate,
		DSO:               dso,
		PromiseToPayRate:  p2p,
		CurrentlyPromised: stats.CurrentlyPromised,
		BadDebtAmount:     aging.Days90Plus,
		DefaultedCount:    stats.DefaultedCount,
		Aging: dto.AgingBreakdown{
			Days0_30:   aging.Days030,
			Days31_60:  aging.Days3160,
			Days61_90:  aging.Days6190,
			Days90Plus: aging.Days90Plus,
		},
	}
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
