package handler

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/cache"
	"cashpilotx/internal/dto"
	mw "cashpilotx/internal/middleware"
	"cashpilotx/internal/service"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Handlers is the main container for all HTTP handlers.
type Handlers struct {
	Auth      *AuthHandler
	Customer  *CustomerHandler
	Invoice   *InvoiceHandler
	Payment   *PaymentHandler
	Dashboard *DashboardHandler
	Message   *MessageHandler
	Reminder  *ReminderHandler
	Audit     *AuditHandler
	SSE       *SSEHandler
	Batch     *BatchHandler
	Sync      *SyncHandler
	Push      *PushHandler
}

// BaseHandler provides common functionality for all handlers.
type BaseHandler struct {
	validate *validator.Validate
	logger   *zerolog.Logger
}

type AuthHandler struct {
	*BaseHandler
	svc *service.AuthService
}

func NewAuthHandler(authService *service.AuthService, base *BaseHandler) *AuthHandler {
	return &AuthHandler{
		BaseHandler: base,
		svc:         authService,
	}
}

// HandlerDeps holds optional dependencies for handler construction.
type HandlerDeps struct {
	// RedisClient is the underlying redis.Client for stream-based handlers.
	// If nil, stream-based handlers (e.g. BatchHandler) will not be created.
	RedisClient *redis.Client
	// RedisCache is the full RedisClient wrapper — used by server-log and WS handlers.
	RedisCache *cache.RedisClient
	// IngestStreamName overrides the Redis stream name for batch ingestion.
	IngestStreamName string
}

func mustCast[T any](svc any, name string) *T {
	v, ok := svc.(*T)
	if !ok {
		panic("invalid " + name + " service type")
	}
	return v
}

// NewHandlers creates all handlers with their service dependencies.
func NewHandlers(
	services *service.Services,
	store db.Store,
	logger *zerolog.Logger,
	deps *HandlerDeps,
) *Handlers {

	v := validator.New()
	if err := api.RegisterCustomValidations(v); err != nil {
		panic(err)
	}

	base := &BaseHandler{
		validate: v,
		logger:   logger,
	}

	authSvc := mustCast[service.AuthService](services.Auth, "auth")
	customerSvc := mustCast[service.CustomerService](services.Customer, "customer")
	invoiceSvc := mustCast[service.InvoiceService](services.Invoice, "invoice")
	paymentSvc := mustCast[service.PaymentService](services.Payment, "payment")
	messageSvc := mustCast[service.MessageService](services.Message, "message")
	reminderSvc := mustCast[service.ReminderService](services.Reminder, "reminder")
	auditSvc := mustCast[service.AuditService](services.Audit, "audit")

	h := &Handlers{
		Auth:      NewAuthHandler(authSvc, base),
		Customer:  NewCustomerHandler(customerSvc, base),
		Invoice:   NewInvoiceHandler(invoiceSvc, base),
		Payment:   NewPaymentHandler(paymentSvc, base),
		Dashboard: NewDashboardHandler(invoiceSvc, base),
		Message:   NewMessageHandler(messageSvc, base),
		Reminder:  NewReminderHandler(reminderSvc, base),
		Audit:     NewAuditHandler(auditSvc, base),
		Sync:      NewSyncHandler(services.Sync, base),
	}

	if deps != nil && deps.RedisClient != nil {
		h.Batch = NewBatchHandler(base, deps.RedisClient, deps.IngestStreamName)
	}
	if deps != nil && deps.RedisCache != nil {
		h.SSE = NewSSEHandler(base, deps.RedisCache)
	}

	h.Push = NewPushHandler(store, base)

	return h
}

// =============================================================================
// Misc handlers
// =============================================================================

func (h *BaseHandler) HandleNotImplement(w http.ResponseWriter, r *http.Request) {
	api.HandleError(w, r, api.NewError(
		api.ErrCodeInternal,
		"This endpoint is not yet implemented",
		http.StatusNotImplemented,
	))
}

func (h *BaseHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	dto.WriteSuccess(w, r, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.21",
	})
}

// =============================================================================
// Request decoding / validation
// =============================================================================

// DecodeJSON decodes the request body into v. It rejects unknown fields.
func (h *BaseHandler) DecodeJSON(r *http.Request, v any) *api.Error {
	if r.Body == nil || r.Body == http.NoBody {
		return api.ErrBadRequest("Request body is empty")
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return api.ErrBadRequest("Request body is empty")
		}
		h.logger.Error().Err(err).
			Str("path", r.URL.Path).
			Msg("JSON decode failed")
		return api.ErrInvalidJSON(err)
	}
	return nil
}

// ValidateRequest validates v against struct tags and returns a rich APIError.
func (h *BaseHandler) ValidateRequest(v any) *api.Error {
	if err := h.validate.Struct(v); err != nil {
		return api.FromValidationError(err)
	}
	return nil
}

// DecodeAndValidate is a convenience helper that decodes and validates a request.
func (h *BaseHandler) DecodeAndValidate(w http.ResponseWriter, r *http.Request, v any) bool {
	if apiErr := h.DecodeJSON(r, v); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return false
	}
	if apiErr := h.ValidateRequest(v); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return false
	}
	return true
}

// =============================================================================
// Error helpers
// =============================================================================

// WriteErr writes an APIError response with request context (request-id, path …).
func (h *BaseHandler) WriteErr(w http.ResponseWriter, r *http.Request, err *api.Error) {
	api.WriteErrorWithContext(w, r, err)
}

// MapErr converts any error (sentinel, APIError, pg, …) to an *api.Error.
func (h *BaseHandler) MapErr(err error) *api.Error {
	if err == nil {
		return nil
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return apiErr
	}

	var valErrs validator.ValidationErrors
	if errors.As(err, &valErrs) {
		return api.FromValidationError(err)
	}

	return api.ErrInternal(err)
}

// HandleErr is a convenience that maps then writes.
func (h *BaseHandler) HandleErr(w http.ResponseWriter, r *http.Request, err error) {
	h.WriteErr(w, r, h.MapErr(err))
}

// =============================================================================
// Context / auth helpers
// =============================================================================

// MustUserID reads the user ID from context; writes 401 and returns false on failure.
func (h *BaseHandler) MustUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := mw.GetUserID(r.Context())
	if raw == "" {
		h.WriteErr(w, r, api.ErrUnauthorized(""))
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrUnauthorized("Malformed user ID in token"))
		return uuid.Nil, false
	}
	return id, true
}

// MustUUIDParam extracts a UUID path parameter. Returns false and writes an
// error response if the parameter is missing or not a valid UUID.
func (h *BaseHandler) MustUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		h.WriteErr(w, r, api.ErrBadRequest("missing "+param+" path parameter"))
		return uuid.Nil, false
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest(param+" must be a valid UUID"))
		return uuid.Nil, false
	}

	return id, true
}

// =============================================================================
// Network helpers
// =============================================================================

func (h *BaseHandler) ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

func (h *BaseHandler) BearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return after
	}
	return ""
}

// parseUUID parses a raw string into a uuid.UUID; used by query-param helpers.
func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

func ParseQueryInt32(r *http.Request, key string, defaultVal int32) int32 {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int32(v)
}

// =============================================================================
// Response helpers
// =============================================================================

// WriteSuccess writes a successful JSON response with status 200.
func (h *BaseHandler) WriteSuccess(w http.ResponseWriter, r *http.Request, data any) {
	dto.WriteSuccess(w, r, data)
}

// WriteCreated writes a successful JSON response with status 201.
func (h *BaseHandler) WriteCreated(w http.ResponseWriter, r *http.Request, data any) {
	dto.WriteCreated(w, r, data)
}

// WriteNoContent writes a 204 No Content response.
func (h *BaseHandler) WriteNoContent(w http.ResponseWriter) {
	dto.WriteNoContent(w)
}

// WriteMessage writes a simple message response.
func (h *BaseHandler) WriteMessage(w http.ResponseWriter, r *http.Request, msg string) {
	dto.WriteSuccess(w, r, dto.NewMessageResponse(msg))
}
