package llm

import (
	"errors"
	"fmt"
)

// Sentinel errors for the LLM package.
var (
	// ErrMissingBaseURL indicates the BaseURL configuration is missing.
	ErrMissingBaseURL = errors.New("llm: base URL is required")

	// ErrMissingModel indicates the model configuration is missing.
	ErrMissingModel = errors.New("llm: model is required")

	// ErrInvalidDimensions indicates the embedding dimensions are invalid.
	ErrInvalidDimensions = errors.New("llm: dimensions must be positive")

	// ErrNoChoices indicates the API returned no completion choices.
	ErrNoChoices = errors.New("llm: no completion choices returned")

	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("llm: circuit breaker open, service unavailable")

	// ErrRateLimited indicates the API rate limit was exceeded.
	ErrRateLimited = errors.New("llm: rate limit exceeded")

	// ErrContextCanceled indicates the context was canceled.
	ErrContextCanceled = errors.New("llm: context canceled")

	// ErrTimeout indicates the request timed out.
	ErrTimeout = errors.New("llm: request timeout")
)

// APIError represents an error from the LLM API.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Body is the response body or error message.
	Body string

	// Code is the error code from the API (if available).
	Code string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("llm API error (status %d, code %s): %s", e.StatusCode, e.Code, e.Body)
	}
	return fmt.Sprintf("llm API error (status %d): %s", e.StatusCode, e.Body)
}

// IsRetryableAPIError returns true if the API error is retryable.
func (e *APIError) IsRetryable() bool {
	// 5xx errors are retryable
	if e.StatusCode >= 500 {
		return true
	}
	// 429 (rate limit) is retryable after waiting
	if e.StatusCode == 429 {
		return true
	}
	return false
}

// IsRateLimited returns true if this is a rate limit error.
func (e *APIError) IsRateLimited() bool {
	return e.StatusCode == 429
}

// IsNotFound returns true if the model or resource was not found.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

// IsAuthError returns true if authentication failed.
func (e *APIError) IsAuthError() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

// IsBadRequest returns true if the request was malformed.
func (e *APIError) IsBadRequest() bool {
	return e.StatusCode == 400
}

// ValidationError represents a validation error for LLM inputs.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("llm validation error: %s: %s", e.Field, e.Message)
}

// SchemaError represents a JSON schema validation error.
type SchemaError struct {
	SchemaName string
	Message    string
	Path       string
}

func (e *SchemaError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("llm schema error (%s at %s): %s", e.SchemaName, e.Path, e.Message)
	}
	return fmt.Sprintf("llm schema error (%s): %s", e.SchemaName, e.Message)
}

// Helper functions for error checking

// IsAPIError checks if an error is an API error.
func IsAPIError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr)
}

// GetAPIError extracts the APIError from an error chain.
func GetAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}

// IsRetryableError checks if an error is retryable.
func IsRetryableError(err error) bool {
	if apiErr := GetAPIError(err); apiErr != nil {
		return apiErr.IsRetryable()
	}
	return false
}

// NewValidationError creates a new validation error.
func NewValidationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

// NewSchemaError creates a new schema error.
func NewSchemaError(schemaName, message string) error {
	return &SchemaError{SchemaName: schemaName, Message: message}
}

// NewSchemaErrorWithPath creates a new schema error with a JSON path.
func NewSchemaErrorWithPath(schemaName, path, message string) error {
	return &SchemaError{SchemaName: schemaName, Path: path, Message: message}
}
