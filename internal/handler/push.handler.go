package handler

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"errors"
	"net/http"
)

// PushHandler manages Web Push subscription storage.
type PushHandler struct {
	*BaseHandler
	store db.Store
}

func NewPushHandler(store db.Store, base *BaseHandler) *PushHandler {
	return &PushHandler{BaseHandler: base, store: store}
}

type subscribePushRequest struct {
	Endpoint string `json:"endpoint" validate:"required"`
	P256DH   string `json:"p256dh"   validate:"required"`
	Auth     string `json:"auth"     validate:"required"`
}

// Subscribe saves or updates a Web Push subscription for the authenticated user.
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req subscribePushRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	sub, err := h.store.UpsertPushSubscription(r.Context(), db.UpsertPushSubscriptionParams{
		UserID:   userID,
		Endpoint: req.Endpoint,
		P256dh:   req.P256DH,
		Auth:     req.Auth,
	})
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID.String()).Msg("push: upsert subscription failed")
		h.WriteErr(w, r, api.ErrInternal(errors.New("failed to save subscription")))
		return
	}

	dto.WriteJSON(w, http.StatusOK, map[string]string{"id": sub.ID.String()})
}

// Unsubscribe removes a Web Push subscription by endpoint.
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Endpoint string `json:"endpoint" validate:"required"`
	}
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	if err := h.store.DeletePushSubscription(r.Context(), db.DeletePushSubscriptionParams{
		UserID:   userID,
		Endpoint: req.Endpoint,
	}); err != nil {
		h.logger.Error().Err(err).Str("user_id", userID.String()).Msg("push: delete subscription failed")
		h.WriteErr(w, r, api.ErrInternal(errors.New("failed to remove subscription")))
		return
	}

	dto.WriteJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
