package errors

import (
	"fmt"
	"net/http"
)

// AppError represents a structured application error with HTTP context.
type AppError struct {
	Code    int    // HTTP status code
	Message string // User-friendly message
	Err     error  // Underlying error (for logging)
	Details string // Additional details for debugging
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// NewAppError creates a new AppError with the given code and message.
func NewAppError(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// WithError adds the underlying error for logging purposes.
func (e *AppError) WithError(err error) *AppError {
	e.Err = err
	return e
}

// WithDetails adds additional debugging information.
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// HTTPStatusCode returns the HTTP status code for this error.
func (e *AppError) HTTPStatusCode() int {
	if e.Code < 100 || e.Code > 599 {
		return http.StatusInternalServerError
	}
	return e.Code
}

// Common application errors
var (
	ErrNotFound = func() *AppError {
		return NewAppError(http.StatusNotFound, "Resource not found")
	}

	ErrUnauthorized = func() *AppError {
		return NewAppError(http.StatusUnauthorized, "Unauthorized")
	}

	ErrForbidden = func() *AppError {
		return NewAppError(http.StatusForbidden, "Access denied")
	}

	ErrBadRequest = func() *AppError {
		return NewAppError(http.StatusBadRequest, "Invalid request")
	}

	ErrConflict = func() *AppError {
		return NewAppError(http.StatusConflict, "Resource already exists")
	}

	ErrInternalServer = func() *AppError {
		return NewAppError(http.StatusInternalServerError, "Internal server error")
	}

	ErrTooManyRequests = func() *AppError {
		return NewAppError(http.StatusTooManyRequests, "Too many requests")
	}
)

// InvalidInput creates a bad request error for invalid input.
func InvalidInput(field, reason string) *AppError {
	return NewAppError(http.StatusBadRequest, "Invalid input").
		WithDetails(fmt.Sprintf("Field %q: %s", field, reason))
}

// DatabaseError creates an internal server error from database errors.
func DatabaseError(err error) *AppError {
	return NewAppError(http.StatusInternalServerError, "Database operation failed").
		WithError(err)
}

// ValidationError creates a bad request error from validation failures.
func ValidationError(msg string) *AppError {
	return NewAppError(http.StatusBadRequest, "Validation failed").
		WithDetails(msg)
}

// NotFoundError creates a not found error with entity info.
func NotFoundError(entity string) *AppError {
	return NewAppError(http.StatusNotFound, fmt.Sprintf("%s not found", entity))
}

// PermissionError creates a forbidden error for access control.
func PermissionError(action string) *AppError {
	return NewAppError(http.StatusForbidden, fmt.Sprintf("Permission denied for action: %s", action))
}
