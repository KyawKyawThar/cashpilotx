package dto

// ErrorResponse is the JSON envelope returned on all API errors.
// Used in swagger @Failure annotations so swag can resolve the type.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the structured error details.
type ErrorBody struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
}

// ErrorDetail provides field-level context on validation failures.
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
