package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/dto"
	"context"

	"github.com/google/uuid"
)

func canSeeCostPrice(role db.UserRole) bool {
	return role == db.UserRoleSuperAdmin || role == db.UserRoleAdmin || role == db.UserRoleAccountant
}

func (s *ProductService) ListProducts(ctx context.Context, orgOwnerID uuid.UUID, role db.UserRole, activeOnly bool) ([]*dto.ProductResponse, error) {
	var products []db.Product
	var err error
	if activeOnly {
		products, err = s.store.ListProducts(ctx, orgOwnerID)
	} else {
		products, err = s.store.ListAllProducts(ctx, orgOwnerID)
	}
	if err != nil {
		return nil, err
	}

	showCost := canSeeCostPrice(role)
	out := make([]*dto.ProductResponse, len(products))
	for i, p := range products {
		out[i] = dto.ToProductResponse(p, showCost)
	}
	return out, nil
}

func (s *ProductService) CreateProduct(ctx context.Context, callerID, orgOwnerID uuid.UUID, req *dto.CreateProductRequest) (*dto.ProductResponse, error) {
	p, err := s.store.CreateProduct(ctx, db.CreateProductParams{
		OrgOwnerID:   orgOwnerID,
		Name:         req.Name,
		Sku:          req.SKU,
		Origin:       db.ProductOrigin(req.Origin),
		Category:     req.Category,
		Unit:         req.Unit,
		CostPrice:    req.CostPrice,
		SellingPrice: req.SellingPrice,
		Currency:     req.Currency,
		CreatedBy:    &callerID,
	})
	if err != nil {
		return nil, err
	}
	return dto.ToProductResponse(p, true), nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, productID, orgOwnerID uuid.UUID, req *dto.UpdateProductRequest) (*dto.ProductResponse, error) {
	var nullOrigin db.NullProductOrigin
	if req.Origin != nil {
		nullOrigin = db.NullProductOrigin{ProductOrigin: db.ProductOrigin(*req.Origin), Valid: true}
	}

	p, err := s.store.UpdateProduct(ctx, db.UpdateProductParams{
		ID:           productID,
		OrgOwnerID:   orgOwnerID,
		Name:         req.Name,
		Sku:          req.SKU,
		Origin:       nullOrigin,
		Category:     req.Category,
		Unit:         req.Unit,
		CostPrice:    req.CostPrice,
		SellingPrice: req.SellingPrice,
		Currency:     req.Currency,
	})
	if err != nil {
		return nil, err
	}
	return dto.ToProductResponse(p, true), nil
}

func (s *ProductService) DeactivateProduct(ctx context.Context, productID, orgOwnerID uuid.UUID) error {
	return s.store.DeactivateProduct(ctx, db.DeactivateProductParams{
		ID:         productID,
		OrgOwnerID: orgOwnerID,
	})
}
