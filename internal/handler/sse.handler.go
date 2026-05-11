package handler

import (
	"cashpilotx/internal/cache"
	"context"
	"fmt"
	"net/http"
	"time"
)

// SSEHandler streams AI draft-ready notifications to the browser over
// Server-Sent Events. It subscribes to the user-scoped Redis Pub/Sub
// channel (cpx:sse:draft_ready:{userID}) that the AI Draft Worker
// publishes to after writing a new message row.
//
// Each event has the shape:
//
//	event: draft_ready
//	data: {"event":"draft_ready","message_id":"<uuid>","invoice_id":"<uuid>"}
//
// The handler is only registered when a RedisClient is present. If the
// dependency is nil the route is not mounted, so callers get 404 instead
// of a silent no-op.
type SSEHandler struct {
	*BaseHandler
	redis *cache.RedisClient
}

func NewSSEHandler(base *BaseHandler, rc *cache.RedisClient) *SSEHandler {
	return &SSEHandler{BaseHandler: base, redis: rc}
}

// ServeSSE godoc
//
//	@Summary      SSE — AI draft ready events
//	@Description  Opens a persistent Server-Sent Events stream scoped to the authenticated user. Emits a "draft_ready" event each time the AI Draft Worker finishes generating a message draft. The client should call GET /api/v1/invoices/{id}/messages after receiving the event to fetch the new draft.
//	@Tags         messages
//	@Produce      text/event-stream
//	@Success      200  {string}  string  "event: draft_ready\\ndata: {…}\\n\\n"
//	@Failure      401  {object}  dto.ErrorResponse
//	@Failure      500  {object}  dto.ErrorResponse
//	@Router       /api/v1/messages/sse [get]
//	@Security     BearerAuth
func (h *SSEHandler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Remove the HTTP server's write deadline for this long-lived connection.
	// Without this the server's WriteTimeout (typically 10 s) would close the
	// SSE stream before the first event arrives.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) //nolint:errcheck // best-effort; not all transports support deadline removal

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx/proxy buffering

	// Subscribe on a non-cancelable context so the Pub/Sub goroutine is not
	// canceled by the request context before we've finished reading from it.
	// Cleanup happens via sub.Close() which drains and terminates the channel.
	sub := h.redis.SubscribeDraftReady(context.WithoutCancel(r.Context()), userID.String())
	defer func() { _ = sub.Close() }() //nolint:errcheck // best-effort cleanup; error is not actionable

	msgCh := sub.Channel()

	// Keepalive comment lines every 25 s prevent proxies / load-balancers from
	// closing an idle connection before any draft_ready event fires.
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	// Confirm the stream is open to the client immediately.
	fmt.Fprintf(w, ": connected user=%s\n\n", userID) //nolint:errcheck,gosec // SSE comment line; write failure detected via r.Context().Done(); userID is a validated UUID
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected or server shutting down — clean exit.
			return

		case <-keepalive.C:
			// SSE comment line; browsers ignore it but it resets idle timers.
			fmt.Fprintf(w, ": ping\n\n") //nolint:errcheck // SSE keepalive; write failure detected via r.Context().Done()
			flusher.Flush()

		case msg, open := <-msgCh:
			if !open {
				// Pub/Sub subscription was closed (Redis disconnect or server restart).
				fmt.Fprintf(w, "event: error\ndata: {\"reason\":\"subscription closed\"}\n\n") //nolint:errcheck // SSE write; failure means client already disconnected
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "event: draft_ready\ndata: %s\n\n", msg.Payload) //nolint:errcheck // SSE write; failure means client already disconnected
			flusher.Flush()
		}
	}
}
