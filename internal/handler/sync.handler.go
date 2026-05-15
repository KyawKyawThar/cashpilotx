package handler

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/service"
	"net/http"
	"time"
)

// SyncHandler serves GET /api/v1/sync.
type SyncHandler struct {
	*BaseHandler
	svc service.SyncServicer
}

func NewSyncHandler(svc service.SyncServicer, base *BaseHandler) *SyncHandler {
	return &SyncHandler{BaseHandler: base, svc: svc}
}

// GetSync godoc
//
//	@Summary      Delta sync
//	@Description  Returns invoices and customers changed since the given RFC3339 cursor. Omit ?since to fetch everything. Use server_time from the response as the next cursor.
//	@Tags         sync
//	@Produce      json
//	@Param        since  query    string  false  "RFC3339 timestamp cursor"  example("2024-04-05T14:32:00Z")
//	@Success      200    {object} dto.SyncResponse
//	@Failure      400    {object} dto.ErrorResponse
//	@Failure      401    {object} dto.ErrorResponse
//	@Router       /api/v1/sync [get]
//	@Security     BearerAuth
func (h *SyncHandler) GetSync(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var since time.Time
	if raw := r.URL.Query().Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			h.WriteErr(w, r, api.ErrBadRequest("since must be an RFC3339 timestamp (e.g. 2024-04-05T14:32:00Z)"))
			return
		}
		since = t
	}
	// since == zero value → returns all records for the user

	data, err := h.svc.GetSyncData(r.Context(), userID, since)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, data)
}
