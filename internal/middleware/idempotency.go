package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	idempotencyHeader    = "Idempotency-Key"
	idempotencyKeyMaxLen = 128
)

// IdempotencyStore is the narrow db.Store surface the middleware needs.
type IdempotencyStore interface {
	GetIdempotencyKey(ctx context.Context, arg db.GetIdempotencyKeyParams) (db.IdempotencyKey, error)
	CreateIdempotencyKey(ctx context.Context, arg db.CreateIdempotencyKeyParams) (db.IdempotencyKey, error)
}

type cachedResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// Idempotency enforces at-most-once semantics for POST requests that carry an
// Idempotency-Key header. On a cache hit the stored response is replayed
// verbatim with an Idempotency-Replayed: true header. Requests without the
// header pass through unchanged.
//
// Must be applied after JWT auth — it reads the user ID from context.
// Only 2xx responses are cached; errors are never stored.
func Idempotency(store IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			clientKey := r.Header.Get(idempotencyHeader)
			if clientKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			storedKey, userID, ok := resolveIdempotencyKey(w, r, clientKey)
			if !ok {
				return
			}

			if replayIdempotentResponse(w, r, store, storedKey, userID) {
				return
			}

			rec := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			rec.flush()

			persistIdempotentResponse(r.Context(), store, storedKey, userID, rec)
		})
	}
}

// resolveIdempotencyKey validates the key length and user context, and returns
// the namespaced stored key and parsed user UUID. Returns ok=false (with error
// already written) if validation fails.
func resolveIdempotencyKey(w http.ResponseWriter, r *http.Request, clientKey string) (string, uuid.UUID, bool) {
	if len(clientKey) > idempotencyKeyMaxLen {
		api.HandleError(w, r, api.ErrBadRequest("Idempotency-Key header exceeds maximum length"))
		return "", uuid.UUID{}, false
	}

	userIDStr := GetUserID(r.Context())
	if userIDStr == "" {
		api.HandleError(w, r, api.ErrUnauthorized(""))
		return "", uuid.UUID{}, false
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		api.HandleError(w, r, api.ErrUnauthorized(""))
		return "", uuid.UUID{}, false
	}

	// Namespace by user: the idempotency_keys PK is TEXT (not composite),
	// so two users with the same client key would collide without this.
	return userIDStr + ":" + clientKey, userID, true
}

// replayIdempotentResponse looks up a cached response and replays it.
// Returns true if the request was served from cache (or an error was written).
// Returns false on a cache miss so the caller can execute the real handler.
func replayIdempotentResponse(w http.ResponseWriter, r *http.Request, store IdempotencyStore, storedKey string, userID uuid.UUID) bool {
	existing, err := store.GetIdempotencyKey(r.Context(), db.GetIdempotencyKeyParams{
		Key:    storedKey,
		UserID: userID,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			api.HandleError(w, r, api.ErrInternal(err))
			return true
		}
		return false // cache miss
	}

	var cached cachedResponse
	if unmarshalErr := json.Unmarshal(existing.Response, &cached); unmarshalErr != nil {
		api.HandleError(w, r, api.ErrInternal(unmarshalErr))
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Idempotency-Replayed", "true")
	w.WriteHeader(cached.Status)
	_, _ = w.Write(cached.Body)
	return true
}

// persistIdempotentResponse stores a 2xx handler response for future replay.
// Non-2xx responses are not cached. Concurrent duplicate inserts are silently
// dropped — both clients received correct responses; the next retry will hit cache.
func persistIdempotentResponse(ctx context.Context, store IdempotencyStore, storedKey string, userID uuid.UUID, rec *responseRecorder) {
	if rec.status < 200 || rec.status >= 300 {
		return
	}

	payload, err := json.Marshal(cachedResponse{
		Status: rec.status,
		Body:   json.RawMessage(rec.body.Bytes()),
	})
	if err != nil {
		return
	}

	_, _ = store.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{ //nolint:errcheck // concurrent duplicate inserts hit the unique constraint; both clients receive correct responses
		Key:      storedKey,
		UserID:   userID,
		Response: payload,
	})
}

// responseRecorder buffers WriteHeader and Write so the middleware can inspect
// the full response before forwarding it to the real ResponseWriter.
// Header() is intentionally NOT overridden: handler calls to w.Header().Set(...)
// flow directly to the real writer's header map and are sent on flush().
type responseRecorder struct {
	http.ResponseWriter
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(b)
}

// flush sends the buffered status and body to the underlying ResponseWriter.
// Call exactly once, after the handler returns.
func (r *responseRecorder) flush() {
	if !r.wroteHeader {
		r.status = http.StatusOK
	}
	r.ResponseWriter.WriteHeader(r.status)
	_, _ = r.ResponseWriter.Write(r.body.Bytes())
}
