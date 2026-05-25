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
	SaleName       string `json:"sale_name"`
	TotalCustomer  int64  `json:"total_customer"`
	CashRevenue    int64  `json:"cash_revenue"`
	InvoiceCount   int64  `json:"invoice_count"`
	InvoiceRevenue int64  `json:"invoice_revenue"`
	TotalOrder     int64  `json:"total_order"`
	TotalValue     int64  `json:"total_value"`
}

// SaleReportRow is shared by Recover Cash, Invoice, Schedule, and Payment sections.
type SaleReportRow struct {
	SaleName   string `json:"sale_name"`
	Count      int64  `json:"count"`
	Amount     int64  `json:"amount"`
	TotalOrder int64  `json:"total_order"`
	TotalValue int64  `json:"total_value"`
}

type ReportTotals struct {
	ProductList ProductListRow `json:"product_list"`
	Sale        SaleRow        `json:"sale"`
	RecoverCash SaleReportRow  `json:"recover_cash"`
	Invoice     SaleReportRow  `json:"invoice"`
	Schedule    SaleReportRow  `json:"schedule"`
	Payment     SaleReportRow  `json:"payment"`
}

type DailyReportResponse struct {
	FromDate    string           `json:"from_date"`
	ToDate      string           `json:"to_date"`
	ProductList []ProductListRow `json:"product_list"`
	Sale        []SaleRow        `json:"sale"`
	RecoverCash []SaleReportRow  `json:"recover_cash"`
	Invoice     []SaleReportRow  `json:"invoice"`
	Schedule    []SaleReportRow  `json:"schedule"`
	Payment     []SaleReportRow  `json:"payment"`
	Totals      ReportTotals     `json:"totals"`
}
