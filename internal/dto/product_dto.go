package dto

import (
	db "cashpilotx/db/sqlc"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Products
// =============================================================================

type CreateProductRequest struct {
	Name         string  `json:"name"          validate:"required,min=1,max=200"`
	SKU          *string `json:"sku"           validate:"omitempty,max=100"`
	Origin       string  `json:"origin"        validate:"required,oneof=thai chinese other"`
	Category     *string `json:"category"      validate:"omitempty,max=100"`
	Unit         *string `json:"unit"          validate:"omitempty,max=50"`
	CostPrice    int64   `json:"cost_price"    validate:"min=0"`
	SellingPrice int64   `json:"selling_price" validate:"min=0"`
	Currency     string  `json:"currency"      validate:"required,len=3"`
}

type UpdateProductRequest struct {
	Name         *string `json:"name"          validate:"omitempty,min=1,max=200"`
	SKU          *string `json:"sku"           validate:"omitempty,max=100"`
	Origin       *string `json:"origin"        validate:"omitempty,oneof=thai chinese other"`
	Category     *string `json:"category"      validate:"omitempty,max=100"`
	Unit         *string `json:"unit"          validate:"omitempty,max=50"`
	CostPrice    *int64  `json:"cost_price"    validate:"omitempty,min=0"`
	SellingPrice *int64  `json:"selling_price" validate:"omitempty,min=0"`
	Currency     *string `json:"currency"      validate:"omitempty,len=3"`
}

type ProductResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	SKU          *string   `json:"sku,omitempty"`
	Origin       string    `json:"origin"`
	Category     *string   `json:"category,omitempty"`
	Unit         *string   `json:"unit,omitempty"`
	CostPrice    *int64    `json:"cost_price,omitempty"` // nil for cs_team / sales
	SellingPrice int64     `json:"selling_price"`
	Currency     string    `json:"currency"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func ToProductResponse(p db.Product, showCost bool) *ProductResponse {
	resp := &ProductResponse{
		ID:           p.ID,
		Name:         p.Name,
		SKU:          p.Sku,
		Origin:       string(p.Origin),
		Category:     p.Category,
		Unit:         p.Unit,
		SellingPrice: p.SellingPrice,
		Currency:     p.Currency,
		IsActive:     p.IsActive,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
	if showCost {
		resp.CostPrice = &p.CostPrice
	}
	return resp
}

// =============================================================================
// Delivery Services — update request (create request is in invoice_dto.go)
// =============================================================================

type UpdateDeliveryServiceRequest struct {
	Name     *string `json:"name"     validate:"omitempty,min=1,max=100"`
	Price    *int64  `json:"price"    validate:"omitempty,min=0"`
	Currency *string `json:"currency" validate:"omitempty,len=3"`
}
