package handler

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type DeliveryServiceHandler struct {
	*BaseHandler
	store db.Store
}

func NewDeliveryServiceHandler(store db.Store, base *BaseHandler) *DeliveryServiceHandler {
	return &DeliveryServiceHandler{BaseHandler: base, store: store}
}

type deliveryServiceResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Price     int64     `json:"price"`
	Currency  string    `json:"currency"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func toDeliveryServiceResponse(s db.DeliveryService) deliveryServiceResponse {
	return deliveryServiceResponse{
		ID:        s.ID,
		Name:      s.Name,
		Price:     s.Price,
		Currency:  s.Currency,
		IsActive:  s.IsActive,
		CreatedAt: s.CreatedAt,
	}
}

// List godoc
//
//	@Summary  List delivery services
//	@Tags     delivery-services
//	@Produce  json
//	@Success  200  {object}  map[string]interface{}
//	@Router   /api/v1/delivery-services [get]
//	@Security BearerAuth
func (h *DeliveryServiceHandler) List(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.MustScope(w, r)
	if !ok {
		return
	}

	rows, err := h.store.ListDeliveryServices(r.Context(), userID)
	if err != nil {
		h.HandleErr(w, r, api.FromPgError(err))
		return
	}

	out := make([]deliveryServiceResponse, len(rows))
	for i, s := range rows {
		out[i] = toDeliveryServiceResponse(s)
	}
	h.WriteSuccess(w, r, out)
}

// Create godoc
//
//	@Summary  Create delivery service
//	@Tags     delivery-services
//	@Accept   json
//	@Produce  json
//	@Success  201  {object}  map[string]interface{}
//	@Router   /api/v1/delivery-services [post]
//	@Security BearerAuth
func (h *DeliveryServiceHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req dto.CreateDeliveryServiceRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	svc, err := h.store.CreateDeliveryService(r.Context(), db.CreateDeliveryServiceParams{
		UserID:    userID,
		Name:      req.Name,
		Price:     req.Price,
		Currency:  req.Currency,
		CreatedBy: &userID,
	})
	if err != nil {
		if api.IsUniqueViolation(err) {
			h.WriteErr(w, r, api.ErrConflict("delivery service with this name already exists"))
			return
		}
		h.HandleErr(w, r, api.FromPgError(err))
		return
	}

	h.WriteCreated(w, r, toDeliveryServiceResponse(svc))
}

// Update godoc
//
//	@Summary  Update delivery service
//	@Tags     delivery-services
//	@Accept   json
//	@Produce  json
//	@Param    service_id  path      string                          true  "Delivery service UUID"
//	@Param    body        body      dto.UpdateDeliveryServiceRequest  true  "Update payload"
//	@Success  200         {object}  map[string]interface{}
//	@Router   /api/v1/delivery-services/{service_id} [patch]
//	@Security BearerAuth
func (h *DeliveryServiceHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	serviceID, ok := h.MustUUIDParam(w, r, "service_id")
	if !ok {
		return
	}

	var req dto.UpdateDeliveryServiceRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	svc, err := h.store.UpdateDeliveryService(r.Context(), db.UpdateDeliveryServiceParams{
		ID:       serviceID,
		UserID:   userID,
		Name:     req.Name,
		Price:    req.Price,
		Currency: req.Currency,
	})
	if err != nil {
		h.HandleErr(w, r, api.FromPgError(err))
		return
	}

	h.WriteSuccess(w, r, toDeliveryServiceResponse(svc))
}

// Delete godoc
//
//	@Summary  Delete delivery service
//	@Tags     delivery-services
//	@Produce  json
//	@Param    service_id  path  string  true  "Delivery service UUID"
//	@Success  204
//	@Router   /api/v1/delivery-services/{service_id} [delete]
//	@Security BearerAuth
func (h *DeliveryServiceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	serviceID, ok := h.MustUUIDParam(w, r, "service_id")
	if !ok {
		return
	}

	if err := h.store.DeleteDeliveryService(r.Context(), db.DeleteDeliveryServiceParams{
		ID:     serviceID,
		UserID: userID,
	}); err != nil {
		h.HandleErr(w, r, api.FromPgError(err))
		return
	}

	h.WriteNoContent(w)
}
