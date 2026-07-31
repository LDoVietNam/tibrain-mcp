// Package api provides REST API error handling
package api

import "fmt"

// APIError represents a structured API error
type APIError struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	Retryable     bool   `json:"retryable,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Details       string `json:"details,omitempty"`
}

// Error returns the error message
func (e *APIError) Error() string {
	return e.Message
}

// NewNotImplemented returns a 501 error
func NewNotImplemented(operation string) *APIError {
	return &APIError{
		Code:      "not_implemented",
		Message:   fmt.Sprintf("%s is not yet implemented", operation),
		Retryable: false,
	}
}

// NewInternalError returns a 500 error
func NewInternalError(details string) *APIError {
	return &APIError{
		Code:      "internal_error",
		Message:   "Internal server error",
		Retryable: true,
		Details:   details,
	}
}
