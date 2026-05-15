package dto

import "time"

// SyncResponse is the payload returned by GET /api/v1/sync.
type SyncResponse struct {
	Invoices   []*InvoiceResponse  `json:"invoices"`
	Customers  []*CustomerResponse `json:"customers"`
	ServerTime time.Time           `json:"server_time"`
}
