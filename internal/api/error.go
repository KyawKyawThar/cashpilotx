// Package api provides HTTP error types, response helpers, and input validators.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"
)

// ErrorCode is a string that represents a specific API error code.
type ErrorCode string

const (
	// ErrCodeValidation indicates a validation error with the request.
	ErrCodeValidation ErrorCode = "VALIDATION_ERROR"
	// ErrCodeUnauthorized indicates that the request is not authenticated.
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrCodeForbidden indicates that the authenticated user does not have permission to perform the action.
	ErrCodeForbidden ErrorCode = "FORBIDDEN"
	// ErrCodeNotFound indicates that the requested resource could not be found.
	ErrCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrCodeConflict indicates a conflict with the current state of the resource.
	ErrCodeConflict ErrorCode = "CONFLICT"
	// ErrCodeUnprocessable indicates that the server understands the content type of the request entity,
	// and the syntax of the request entity is correct but was unable to process the contained instructions.
	ErrCodeUnprocessable ErrorCode = "UNPROCESSABLE_ENTITY"
	// ErrCodeRateLimited indicates that the user has sent too many requests in a given amount of time.
	ErrCodeRateLimited ErrorCode = "RATE_LIMITED"
	// ErrCodeBadRequest indicates that the server cannot or will not process the request due to something that is perceived to be a client error.
	ErrCodeBadRequest ErrorCode = "BAD_REQUEST"
	// ErrCodePayloadTooLarge indicates that the request is larger than the server is willing or able to process.
	ErrCodePayloadTooLarge ErrorCode = "PAYLOAD_TOO_LARGE"

	// ErrCodeInternal indicates an internal server error.
	ErrCodeInternal ErrorCode = "INTERNAL_ERROR"
	// ErrCodeUnavailable indicates that the server is not ready to handle the request.
	ErrCodeUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	// ErrCodeTimeout indicates that the server did not receive a timely response from an upstream server.
	ErrCodeTimeout ErrorCode = "TIMEOUT"
	// ErrCodeDatabase indicates a database error.
	ErrCodeDatabase ErrorCode = "DATABASE_ERROR"

	// CashPilotX auth errors
	ErrCodeInvalidToken       ErrorCode = "INVALID_TOKEN"
	ErrCodeExpiredToken       ErrorCode = "EXPIRED_TOKEN"
	ErrCodeInvalidCredentials ErrorCode = "INVALID_CREDENTIALS" //nolint:gosec // not a credential
	ErrCodeSessionExpired     ErrorCode = "SESSION_EXPIRED"
	ErrCodeLoginLocked        ErrorCode = "LOGIN_LOCKED"

	// CashPilotX domain errors
	ErrCodePhoneAlreadyExists     ErrorCode = "PHONE_ALREADY_EXISTS"
	ErrCodeEmailAlreadyExists     ErrorCode = "EMAIL_ALREADY_EXISTS"
	ErrCodeInvalidOTP             ErrorCode = "INVALID_OTP"
	ErrCodeInvalidStateTransition ErrorCode = "INVALID_STATE_TRANSITION"
	ErrCodeInvoiceOverpaid        ErrorCode = "INVOICE_OVERPAID"
	ErrCodeActiveReminderExists   ErrorCode = "ACTIVE_REMINDER_EXISTS"
)

// ErrorDetail provides more specific information about an error.
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Value   any    `json:"value,omitempty"`
}

// Error represents a structured error response.
type Error struct {
	Code      ErrorCode     `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	RequestID string        `json:"request_id,omitempty"`

	// Internal fields (not serialized to JSON)
	HTTPStatus int       `json:"-"`
	Internal   error     `json:"-"`
	Stack      string    `json:"-"`
	Timestamp  time.Time `json:"-"`
	Path       string    `json:"-"`
	Method     string    `json:"-"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Internal
}

// =============================================================================
// Chainable Methods (Builder Pattern)
// =============================================================================

// WithRequestID adds a request ID to the error.
func (e *Error) WithRequestID(requestID string) *Error {
	e.RequestID = requestID
	return e
}

// WithPath adds the HTTP method and path to the error.
func (e *Error) WithPath(method, path string) *Error {
	e.Method = method
	e.Path = path
	return e
}

// WithDetails adds details to the error.
func (e *Error) WithDetails(details ...ErrorDetail) *Error {
	e.Details = append(e.Details, details...)
	return e
}

// WithDetail adds a single detail to the error.
func (e *Error) WithDetail(field, message string) *Error {
	e.Details = append(e.Details, ErrorDetail{Field: field, Message: message})
	return e
}

// WithInternal adds an internal error.
func (e *Error) WithInternal(err error) *Error {
	e.Internal = err
	return e
}

// WithStack adds a stack trace to the error.
func (e *Error) WithStack() *Error {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	e.Stack = string(buf[:n])
	return e
}

// =============================================================================
// Response Methods
// =============================================================================

// ToResponse converts the error to a map suitable for a JSON response.
func (e *Error) ToResponse() map[string]any {
	response := map[string]any{
		"code":    e.Code,
		"message": e.Message,
	}
	if len(e.Details) > 0 {
		response["details"] = e.Details
	}
	if e.RequestID != "" {
		response["request_id"] = e.RequestID
	}
	return map[string]any{"error": response}
}

// ToJSON converts the error to a JSON byte slice.
func (e *Error) ToJSON() []byte {
	data, err := json.Marshal(e.ToResponse())
	if err != nil {
		log.Error().Err(err).Msg("could not marshal error response")
		return []byte(`{"error":{"code":"INTERNAL_ERROR","message":"Failed to serialize error response"}}`)
	}
	return data
}

// LogFields returns fields for structured logging
func (e *Error) LogFields() map[string]any {
	fields := map[string]any{
		"error_code":    e.Code,
		"error_message": e.Message,
		"http_status":   e.HTTPStatus,
	}
	if e.RequestID != "" {
		fields["request_id"] = e.RequestID
	}
	if e.Path != "" {
		fields["path"] = e.Path
		fields["method"] = e.Method
	}
	if e.Internal != nil {
		fields["internal_error"] = e.Internal.Error()
	}
	return fields
}

// =============================================================================
// Error Constructor
// =============================================================================

// NewError creates a new Error.
func NewError(code ErrorCode, message string, httpStatus int) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Timestamp:  time.Now().UTC(),
	}
}

// =============================================================================
// Client Error Constructors (4xx)
// =============================================================================

// --- 400 Bad Request ---

// ErrBadRequest creates a new 400 Bad Request error.
func ErrBadRequest(message string) *Error {
	return NewError(ErrCodeBadRequest, message, http.StatusBadRequest)
}

// ErrValidation creates a new 400 Validation Error.
func ErrValidation(message string) *Error {
	return NewError(ErrCodeValidation, message, http.StatusBadRequest)
}

// ErrInvalidJSON creates a new 400 Invalid JSON error.
func ErrInvalidJSON(err error) *Error {
	return NewError(ErrCodeValidation, "Invalid JSON payload", http.StatusBadRequest).
		WithInternal(err)
}

// ErrInvalidQueryParam creates a new 400 Invalid Query Param error.
func ErrInvalidQueryParam(param, message string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Invalid query parameter '%s': %s", param, message), http.StatusBadRequest).
		WithDetail(param, message)
}

// ErrInvalidPathParam creates a new 400 Invalid Path Param error.
func ErrInvalidPathParam(param, message string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Invalid path parameter '%s': %s", param, message), http.StatusBadRequest).
		WithDetail(param, message)
}

// ErrInvalidUUID creates a new 400 Invalid UUID error.
func ErrInvalidUUID(field string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Invalid UUID format for '%s'", field), http.StatusBadRequest).
		WithDetail(field, "must be a valid UUID")
}

// ErrMissingRequired creates a new 400 Missing Required Field error.
func ErrMissingRequired(field string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Missing required field '%s'", field), http.StatusBadRequest).
		WithDetail(field, "is required")
}

// --- 401 Unauthorized ---

// ErrUnauthorized creates a new 401 Unauthorized error.
func ErrUnauthorized(message string) *Error {
	if message == "" {
		message = "Authentication required"
	}
	return NewError(ErrCodeUnauthorized, message, http.StatusUnauthorized)
}

// ErrInvalidToken creates a new 401 Invalid Token error.
func ErrInvalidToken(message string) *Error {
	if message == "" {
		message = "Invalid or malformed token"
	}
	return NewError(ErrCodeInvalidToken, message, http.StatusUnauthorized)
}

// ErrExpiredToken creates a new 401 Expired Token error.
func ErrExpiredToken() *Error {
	return NewError(ErrCodeExpiredToken, "Token has expired", http.StatusUnauthorized)
}

// ErrInvalidCredentials creates a new 401 Invalid Credentials error.
func ErrInvalidCredentials() *Error {
	return NewError(ErrCodeInvalidCredentials, "Invalid phone number or PIN", http.StatusUnauthorized)
}

// ErrSessionExpired creates a new 401 Session Expired error.
func ErrSessionExpired() *Error {
	return NewError(ErrCodeSessionExpired, "Session has expired, please log in again", http.StatusUnauthorized)
}

// ErrMissingAuthHeader creates a new 401 Missing Auth Header error.
func ErrMissingAuthHeader() *Error {
	return NewError(ErrCodeUnauthorized, "Missing Authorization header", http.StatusUnauthorized)
}

// ErrInvalidAuthHeaderFormat creates a new 401 Invalid Auth Header Format error.
func ErrInvalidAuthHeaderFormat() *Error {
	return NewError(ErrCodeUnauthorized, "Invalid Authorization header format", http.StatusUnauthorized)
}

// ErrUnsupportedAuthType creates a new 401 Unsupported Auth Type error.
func ErrUnsupportedAuthType(authType string) *Error {
	return NewError(ErrCodeUnauthorized, "Unsupported authorization type", http.StatusUnauthorized).
		WithDetail("type", "unsupported authorization type, only "+authType+" is supported")
}

// ErrLoginLocked creates a new 401 Login Locked error (brute-force lockout).
func ErrLoginLocked(lockedUntil time.Time) *Error {
	return NewError(ErrCodeLoginLocked,
		fmt.Sprintf("Account locked due to too many failed attempts. Try again after %s", lockedUntil.UTC().Format(time.RFC3339)),
		http.StatusUnauthorized,
	).WithDetail("locked_until", lockedUntil.UTC().Format(time.RFC3339))
}

// --- 403 Forbidden ---

// ErrForbidden creates a new 403 Forbidden error.
func ErrForbidden(message string) *Error {
	if message == "" {
		message = "You don't have permission to perform this action"
	}
	return NewError(ErrCodeForbidden, message, http.StatusForbidden)
}

// --- 404 Not Found ---

// ErrNotFound creates a new 404 Not Found error.
func ErrNotFound(resource string) *Error {
	return NewError(ErrCodeNotFound, fmt.Sprintf("%s not found", resource), http.StatusNotFound)
}

// ErrUserNotFound creates a new 404 User Not Found error.
func ErrUserNotFound() *Error {
	return NewError(ErrCodeNotFound, "User not found", http.StatusNotFound)
}

// ErrInvoiceNotFound creates a new 404 Invoice Not Found error.
func ErrInvoiceNotFound() *Error {
	return NewError(ErrCodeNotFound, "Invoice not found", http.StatusNotFound)
}

// ErrCustomerNotFound creates a new 404 Customer Not Found error.
func ErrCustomerNotFound() *Error {
	return NewError(ErrCodeNotFound, "Customer not found", http.StatusNotFound)
}

// ErrPaymentNotFound creates a new 404 Payment Not Found error.
func ErrPaymentNotFound() *Error {
	return NewError(ErrCodeNotFound, "Payment not found", http.StatusNotFound)
}

// ErrReminderNotFound creates a new 404 Reminder Not Found error.
func ErrReminderNotFound() *Error {
	return NewError(ErrCodeNotFound, "Reminder not found", http.StatusNotFound)
}

// ErrMessageNotFound creates a new 404 Message Not Found error.
func ErrMessageNotFound() *Error {
	return NewError(ErrCodeNotFound, "Message not found", http.StatusNotFound)
}

// --- 409 Conflict ---

// ErrConflict creates a new 409 Conflict error.
func ErrConflict(message string) *Error {
	return NewError(ErrCodeConflict, message, http.StatusConflict)
}

// ErrPhoneAlreadyExists creates a new 409 Phone Already Exists error.
func ErrPhoneAlreadyExists() *Error {
	return NewError(ErrCodePhoneAlreadyExists, "Phone number is already registered", http.StatusConflict).
		WithDetail("phone", "already exists")
}

// ErrEmailAlreadyExists creates a new 409 Email Already Exists error.
func ErrEmailAlreadyExists() *Error {
	return NewError(ErrCodeEmailAlreadyExists, "Email address is already registered", http.StatusConflict).
		WithDetail("email", "already exists")
}

// ErrInvalidOTP creates a new 400 Invalid OTP error.
func ErrInvalidOTP() *Error {
	return NewError(ErrCodeInvalidOTP, "Invalid or expired OTP code", http.StatusBadRequest).
		WithDetail("otp", "invalid or expired")
}

// ErrActiveReminderExists creates a new 409 Active Reminder Exists error.
// Raised when trying to schedule a second active reminder for the same invoice.
func ErrActiveReminderExists() *Error {
	return NewError(ErrCodeActiveReminderExists, "This invoice already has a pending or processing reminder", http.StatusConflict).
		WithDetail("invoice_id", "active reminder exists")
}

// ErrDuplicateResource creates a new 409 Duplicate Resource error.
func ErrDuplicateResource(resource, field string) *Error {
	return NewError(ErrCodeConflict, fmt.Sprintf("%s with this %s already exists", resource, field), http.StatusConflict).
		WithDetail(field, "already exists")
}

// --- 422 Unprocessable Entity ---

// ErrUnprocessable creates a new 422 Unprocessable Entity error.
func ErrUnprocessable(message string) *Error {
	return NewError(ErrCodeUnprocessable, message, http.StatusUnprocessableEntity)
}

// ErrInvalidStateTransition creates a new 422 error for an illegal collection-state move.
func ErrInvalidStateTransition(from, to string) *Error {
	return NewError(ErrCodeInvalidStateTransition,
		fmt.Sprintf("Cannot transition collection state from '%s' to '%s'", from, to),
		http.StatusUnprocessableEntity,
	).WithDetail("collection_state", fmt.Sprintf("'%s' → '%s' is not a valid transition", from, to))
}

// ErrInvoiceOverpaid creates a new 422 error when a payment would exceed the invoice amount.
func ErrInvoiceOverpaid() *Error {
	return NewError(ErrCodeInvoiceOverpaid, "Payment amount exceeds the remaining invoice balance", http.StatusUnprocessableEntity).
		WithDetail("amount", "exceeds remaining balance")
}

// ErrInvalidReference creates a new 422 Invalid Reference error.
func ErrInvalidReference(field, reference string) *Error {
	return NewError(ErrCodeUnprocessable, fmt.Sprintf("Invalid reference in '%s': %s does not exist", field, reference), http.StatusUnprocessableEntity).
		WithDetail(field, "references non-existent record")
}

// --- 429 Rate Limited ---

// ErrRateLimited creates a new 429 Rate Limited error.
func ErrRateLimited(retryAfter int) *Error {
	return NewError(ErrCodeRateLimited, fmt.Sprintf("Rate limit exceeded. Retry after %d seconds", retryAfter), http.StatusTooManyRequests)
}

// =============================================================================
// Server Error Constructors (5xx)
// =============================================================================

// ErrInternal creates a new 500 Internal Server Error.
func ErrInternal(err error) *Error {
	return NewError(ErrCodeInternal, "An internal error occurred", http.StatusInternalServerError).
		WithInternal(err).
		WithStack()
}

// ErrDatabase creates a new 500 Database Error.
func ErrDatabase(err error) *Error {
	return NewError(ErrCodeDatabase, "A database error occurred", http.StatusInternalServerError).
		WithInternal(err).
		WithStack()
}

// ErrUnavailable creates a new 503 Service Unavailable error.
func ErrUnavailable(message string) *Error {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	return NewError(ErrCodeUnavailable, message, http.StatusServiceUnavailable)
}

// ErrDatabaseUnavailable creates a new 503 Database Unavailable error.
func ErrDatabaseUnavailable() *Error {
	return NewError(ErrCodeUnavailable, "Database is temporarily unavailable", http.StatusServiceUnavailable)
}

// ErrTimeout creates a new 504 Gateway Timeout error.
func ErrTimeout(operation string) *Error {
	return NewError(ErrCodeTimeout, fmt.Sprintf("Operation timed out: %s", operation), http.StatusGatewayTimeout)
}

// =============================================================================
// HTTP Response Helpers
//

// WriteError writes an Error to the response.
func WriteError(w http.ResponseWriter, err *Error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(err.HTTPStatus)
	if err := json.NewEncoder(w).Encode(err.ToResponse()); err != nil {
		log.Error().Err(err).Msg("could not write error response")
	}
}

// WriteErrorWithContext writes an Error to the response with context.
func WriteErrorWithContext(w http.ResponseWriter, r *http.Request, err *Error) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}
	w.Header().Set("X-Request-ID", requestID)
	err = err.WithRequestID(requestID).WithPath(r.Method, r.URL.Path)
	WriteError(w, err)
}

// HandleError handles an error and writes it to the response.
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := ToError(err)
	WriteErrorWithContext(w, r, apiErr)
}

// ToError converts an error to an Error.
func ToError(err error) *Error {
	if err == nil {
		return nil
	}
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr
	}
	return ErrInternal(err)
}
