package dto

import "time"

type ReportFilter struct {
	FromDate     time.Time `json:"from_date"`
	ToDate       time.Time `json:"to_date"`
	TrackingCode string    `json:"tracking_code,omitempty"`
	ProductName  string    `json:"product_name,omitempty"`
	SaleName     string    `json:"sale_name,omitempty"`
}

type ProductListRow struct {
	ProductName string `json:"product_name"`
	TotalQty    int64  `json:"total_qty"`
	Revenue     int64  `json:"revenue"`
	TotalOrder  int64  `json:"total_order"`
	TotalValue  int64  `json:"total_value"`
}

type SaleRow struct {
	SaleName          string `json:"sale_name"`
	TotalCustomer     int64  `json:"total_customer"`
	CustomersSubtotal int64  `json:"customers_subtotal"`
	CashRevenue       int64  `json:"cash_revenue"`
	InvoiceCount      int64  `json:"invoice_count"`
	InvoiceRevenue    int64  `json:"invoice_revenue"`
	TotalOrder        int64  `json:"total_order"`
	TotalValue        int64  `json:"total_value"`
}

// SaleReportRow is shared by Recover Cash, Invoice, Schedule, and Payment sections.
// CollectedAmount and OutstandingAmount are only populated for the Invoice section.
type SaleReportRow struct {
	SaleName          string `json:"sale_name"`
	Count             int64  `json:"count"`
	Amount            int64  `json:"amount"`
	CollectedAmount   int64  `json:"collected_amount"`
	OutstandingAmount int64  `json:"outstanding_amount"`
	TotalOrder        int64  `json:"total_order"`
	TotalValue        int64  `json:"total_value"`
}

type ReportTotals struct {
	ProductList ProductListRow `json:"product_list"`
	Sale        SaleRow        `json:"sale"`
	RecoverCash SaleReportRow  `json:"recover_cash"`
	Invoice     SaleReportRow  `json:"invoice"`
	Schedule    SaleReportRow  `json:"schedule"`
	Payment     SaleReportRow  `json:"payment"`
}

type AgingBreakdown struct {
	Days0_30   int64 `json:"days_0_30"`
	Days31_60  int64 `json:"days_31_60"`
	Days61_90  int64 `json:"days_61_90"`
	Days90Plus int64 `json:"days_90_plus"`
}

type ReportSummary struct {
	TotalInvoiced     int64          `json:"total_invoiced"`
	TotalOutstanding  int64          `json:"total_outstanding"`
	CollectionRate    float64        `json:"collection_rate"`
	DSO               float64        `json:"dso"`
	PromiseToPayRate  float64        `json:"promise_to_pay_rate"`
	CurrentlyPromised int64          `json:"currently_promised"`
	BadDebtAmount     int64          `json:"bad_debt_amount"`
	DefaultedCount    int64          `json:"defaulted_count"`
	Aging             AgingBreakdown `json:"aging"`
}

type OverdueCustomerRow struct {
	CustomerName      string `json:"customer_name"`
	Phone             string `json:"phone"`
	SaleName          string `json:"sale_name"`
	InvoiceCount      int64  `json:"invoice_count"`
	OutstandingAmount int64  `json:"outstanding_amount"`
	MaxDaysOverdue    int64  `json:"max_days_overdue"`
}

type DailyReportResponse struct {
	FromDate            string               `json:"from_date"`
	ToDate              string               `json:"to_date"`
	Summary             ReportSummary        `json:"summary"`
	TopOverdueCustomers []OverdueCustomerRow `json:"top_overdue_customers"`
	ProductList         []ProductListRow     `json:"product_list"`
	Sale                []SaleRow            `json:"sale"`
	RecoverCash         []SaleReportRow      `json:"recover_cash"`
	Invoice             []SaleReportRow      `json:"invoice"`
	Schedule            []SaleReportRow      `json:"schedule"`
	Payment             []SaleReportRow      `json:"payment"`
	Totals              ReportTotals         `json:"totals"`
}
