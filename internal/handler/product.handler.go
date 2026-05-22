package handler

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"cashpilotx/internal/service"
	"net/http"
)

type ProductHandler struct {
	*BaseHandler
	svc service.ProductServicer
}

func NewProductHandler(svc service.ProductServicer, base *BaseHandler) *ProductHandler {
	return &ProductHandler{BaseHandler: base, svc: svc}
}

// List godoc
//
//	@Summary      List products
//	@Description  Returns all products for the org. cost_price is hidden for cs_team and sales.
//	@Tags         products
//	@Produce      json
//	@Param        active  query     bool  false  "Filter active only (default true)"
//	@Success      200     {array}   dto.ProductResponse
//	@Failure      401     {object}  dto.ErrorResponse
//	@Router       /api/v1/products [get]
//	@Security     BearerAuth
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	activeOnly := r.URL.Query().Get("active") != "false"

	products, err := h.svc.ListProducts(r.Context(), effectiveUserID, scope.CallerRole, activeOnly)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, products)
}

// Create godoc
//
//	@Summary      Create product
//	@Description  Creates a new product. Admin or Super Admin only.
//	@Tags         products
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateProductRequest  true  "Product payload"
//	@Success      201   {object}  dto.ProductResponse
//	@Failure      400   {object}  dto.ErrorResponse
//	@Failure      401   {object}  dto.ErrorResponse
//	@Failure      403   {object}  dto.ErrorResponse
//	@Router       /api/v1/products [post]
//	@Security     BearerAuth
func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	scope, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	var req dto.CreateProductRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	product, err := h.svc.CreateProduct(r.Context(), scope.CallerID, effectiveUserID, &req)
	if err != nil {
		if api.IsUniqueViolation(err) {
			h.WriteErr(w, r, api.ErrConflict("product with this SKU already exists"))
			return
		}
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, product)
}

// Update godoc
//
//	@Summary      Update product
//	@Description  Updates product fields. Admin or Super Admin only.
//	@Tags         products
//	@Accept       json
//	@Produce      json
//	@Param        product_id  path      string                   true  "Product UUID"
//	@Param        body        body      dto.UpdateProductRequest  true  "Update payload"
//	@Success      200         {object}  dto.ProductResponse
//	@Failure      400         {object}  dto.ErrorResponse
//	@Failure      401         {object}  dto.ErrorResponse
//	@Failure      403         {object}  dto.ErrorResponse
//	@Router       /api/v1/products/{product_id} [patch]
//	@Security     BearerAuth
func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	productID, ok := h.MustUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	var req dto.UpdateProductRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	product, err := h.svc.UpdateProduct(r.Context(), productID, effectiveUserID, &req)
	if err != nil {
		if api.IsUniqueViolation(err) {
			h.WriteErr(w, r, api.ErrConflict("product with this SKU already exists"))
			return
		}
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, product)
}

// Deactivate godoc
//
//	@Summary      Deactivate product
//	@Description  Soft-deletes a product (is_active = false). Admin or Super Admin only.
//	@Tags         products
//	@Produce      json
//	@Param        product_id  path  string  true  "Product UUID"
//	@Success      204
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      403  {object}  dto.ErrorResponse
//	@Router       /api/v1/products/{product_id} [delete]
//	@Security     BearerAuth
func (h *ProductHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	_, effectiveUserID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	productID, ok := h.MustUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.svc.DeactivateProduct(r.Context(), productID, effectiveUserID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}
