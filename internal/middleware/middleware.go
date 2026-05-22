package middleware

import (
	"cashpilotx/internal/api"
	"cashpilotx/internal/service"
	"cashpilotx/internal/token"
	"context"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// =============================================================================
// Context Keys
// =============================================================================

type contextKey string

const (
	ContextKeyRequestID   contextKey = "request_id"
	ContextKeyRequestTime contextKey = "request_time"
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyPayload     contextKey = "jwt_payload"
)

// =============================================================================
// Context Getters / Setters
// =============================================================================

func GetRequestID(ctx context.Context) string {
	id, ok := ctx.Value(ContextKeyRequestID).(string)
	if !ok {
		return ""
	}
	return id
}

func GetRequestTime(ctx context.Context) time.Time {
	t, ok := ctx.Value(ContextKeyRequestTime).(time.Time)
	if !ok {
		return time.Time{}
	}
	return t
}

func GetUserID(ctx context.Context) string {
	id, ok := ctx.Value(ContextKeyUserID).(string)
	if !ok {
		return ""
	}
	return id
}

func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ContextKeyRequestID, requestID)
}

func SetUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

func GetPayload(ctx context.Context) *token.Payload {
	p, _ := ctx.Value(ContextKeyPayload).(*token.Payload) //nolint:errcheck // type assertion; nil is the correct zero value
	return p
}

func setPayload(ctx context.Context, p *token.Payload) context.Context {
	return context.WithValue(ctx, ContextKeyPayload, p)
}

// =============================================================================
// Request ID
// =============================================================================

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		ctx = context.WithValue(ctx, ContextKeyRequestTime, time.Now())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// =============================================================================
// Recoverer
// =============================================================================

func Recoverer(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func(ctx context.Context) {
				if err := recover(); err != nil {
					requestID := GetRequestID(ctx)
					stack := string(debug.Stack())
					logger.Error().
						Interface("error", err).
						Str("request_id", requestID).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("stack", stack).
						Msg("panic recovered")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					if encErr := json.NewEncoder(w).Encode(map[string]any{
						"error": map[string]any{
							"code":       "INTERNAL_ERROR",
							"message":    "An internal error occurred",
							"request_id": requestID,
						},
					}); encErr != nil {
						logger.Error().Err(encErr).Msg("failed to write panic response")
					}
				}
			}(r.Context())
			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Security Headers
// =============================================================================

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Content-Type enforcement
// =============================================================================

func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			ct := r.Header.Get("Content-Type")
			if ct != "" && !strings.HasPrefix(ct, "application/json") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_ = json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // best-effort write; connection may already be closed
					"error": map[string]any{
						"code":    "UNSUPPORTED_MEDIA_TYPE",
						"message": "Content-Type must be application/json",
					},
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Timeout
// =============================================================================

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClearTimeout strips the deadline from the context so long-lived connections
// (e.g. WebSocket) are not killed by the global request timeout.
func ClearTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithoutCancel(r.Context())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// =============================================================================
// Real IP
// =============================================================================

func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			if idx := strings.IndexByte(ip, ','); idx != -1 {
				ip = ip[:idx]
			}
			r.RemoteAddr = strings.TrimSpace(ip)
		} else if ip := r.Header.Get("X-Real-IP"); ip != "" {
			r.RemoteAddr = ip
		} else if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Max Body Size
// =============================================================================

func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// JWT Auth
// =============================================================================

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	after, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

// JWTAuth validates a Bearer token with a token.Maker (no blacklist check).
// Use on internal or low-risk routes where Redis is not available.
func JWTAuth(maker token.Maker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearerToken(r)
			if tok == "" {
				api.WriteErrorWithContext(w, r, api.ErrUnauthorized(""))
				return
			}
			payload, err := maker.VerifyToken(tok)
			if err != nil {
				if strings.Contains(err.Error(), "expired") {
					api.WriteErrorWithContext(w, r, api.ErrExpiredToken())
					return
				}
				api.WriteErrorWithContext(w, r, api.ErrInvalidToken(""))
				return
			}
			ctx := SetUserID(r.Context(), payload.UserID.String())
			ctx = setPayload(ctx, payload)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// JWTAuthWithService validates a Bearer token via the auth service, which also
// checks the Redis blacklist. Use this on all standard protected routes.
func JWTAuthWithService(svc service.AuthServicer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearerToken(r)
			if tok == "" {
				api.WriteErrorWithContext(w, r, api.ErrUnauthorized(""))
				return
			}
			payload, err := svc.ValidateAccessToken(r.Context(), tok)
			if err != nil {
				api.WriteErrorWithContext(w, r, api.ErrUnauthorized(""))
				return
			}
			ctx := SetUserID(r.Context(), payload.UserID.String())
			ctx = setPayload(ctx, payload)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
